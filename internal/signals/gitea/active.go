package gitea

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/codyconfer/mino/internal/auth"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/signals"
	"github.com/codyconfer/mino/internal/signals/active"
)

const (
	notificationsPath   = "/notifications"
	notificationLimit   = 50
	notificationMaxPage = 5
)

type activeSignal struct {
	src      auth.TokenSource
	baseURL  string
	interval time.Duration
	http     *http.Client
	state    *active.State
}

func NewActive(src auth.TokenSource, baseURL string, interval time.Duration, state *active.State) signals.ActiveSignal {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &activeSignal{src: src, baseURL: baseURL, interval: interval, http: signals.HTTPClient(), state: state}
}

func (h *activeSignal) Name() string { return "gitea" }

func (h *activeSignal) LatencyFloor() time.Duration { return h.interval }

func (h *activeSignal) client() *http.Client {
	if h.http == nil {
		return signals.HTTPClient()
	}
	return h.http
}

func (h *activeSignal) Stream(ctx context.Context) (<-chan signals.Event, error) {
	return active.PollAdaptive(ctx, "gitea", h.interval, h.step(ctx)), nil
}

func (h *activeSignal) step(ctx context.Context) func(context.Context) ([]signals.Item, time.Duration, error) {
	cursor := h.state.Cursor("gitea", "since")
	seen := h.state.Seen("gitea:notifications")
	since := cursor.Load(ctx)
	fails := 0
	return func(ctx context.Context) ([]signals.Item, time.Duration, error) {
		res, err := h.poll(ctx, since)
		if err != nil {
			fails++
			return nil, h.retryInterval(res.next, fails), err
		}
		fails = 0
		if res.since != "" && res.since != since {
			since = res.since
			_ = cursor.Save(ctx, res.since)
		}
		return seen.Unseen(ctx, res.items, notificationKey), res.next, nil
	}
}

func notificationKey(it signals.Item) string {
	return it.Meta["id"] + "|" + it.Meta["updated"]
}

type pollResult struct {
	items []signals.Item
	next  time.Duration
	since string
}

func partial(res pollResult) pollResult {
	res.since = ""
	return res
}

func (h *activeSignal) poll(ctx context.Context, since string) (pollResult, error) {
	res := pollResult{next: h.interval}
	newest := since
	for page := 1; page <= notificationMaxPage; page++ {
		pg, err := h.fetchPage(ctx, since, page)
		if err != nil {
			if len(res.items) > 0 {
				log.Debugf("gitea: notifications page %d failed after %d thread(s); discarding the poll: %v",
					page, len(res.items), err)
				res.next = max(res.next, pg.next)
			} else {
				res.next = pg.next
			}
			return partial(res), err
		}
		if page == 1 {
			res.next = pg.next
		}
		items, latest, err := mapNotifications(pg.body)
		if err != nil {
			if len(res.items) > 0 {
				log.Debugf("gitea: notifications page %d unreadable after %d thread(s); discarding the poll: %v",
					page, len(res.items), err)
			}
			return partial(res), err
		}
		res.items = append(res.items, items...)
		if latest.After(parseTime(newest)) {
			newest = latest.UTC().Format(time.RFC3339)
		}
		if pg.count < notificationLimit {
			break
		}
		if page == notificationMaxPage {
			log.Debugf("gitea: stopping after %d notification page(s); %d thread(s) collected and more remain",
				notificationMaxPage, len(res.items))
		}
	}
	res.since = newest
	return res, nil
}

type notificationPage struct {
	body  []byte
	count int
	next  time.Duration
}

func (h *activeSignal) fetchPage(ctx context.Context, since string, page int) (notificationPage, error) {
	pg := notificationPage{next: h.interval}
	tok, err := h.src.Token(ctx)
	if err != nil {
		return pg, err
	}
	req, err := newGiteaRequest(ctx, h.baseURL, notificationQuery(since, page), tok)
	if err != nil {
		return pg, errs.Wrap(errs.KindSignal, err, "gitea: building the notifications request")
	}
	resp, err := h.client().Do(req)
	if err != nil {
		return pg, errs.Wrap(errs.KindSignal, err, "gitea: the notifications request failed")
	}
	defer resp.Body.Close()
	pg.next = nextPollInterval(h.interval, resp)
	body, err := readBody(resp)
	if err != nil {
		return pg, err
	}
	if err := checkGiteaStatus(resp, body, scopeNotify); err != nil {
		return pg, err
	}
	pg.body = body
	pg.count = countJSONArray(body)
	return pg, nil
}

func notificationQuery(since string, page int) string {
	q := fmt.Sprintf("%s?all=false&status-types=unread&limit=%d&page=%d", notificationsPath, notificationLimit, page)
	if since != "" {
		q += "&since=" + since
	}
	return q
}

func (h *activeSignal) retryInterval(next time.Duration, fails int) time.Duration {
	return max(next, withJitter(backoffInterval(h.interval, fails)))
}

func countJSONArray(raw []byte) int {
	var probe []json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return 0
	}
	return len(probe)
}

type notification struct {
	ID      json.Number `json:"id"`
	Unread  bool        `json:"unread"`
	Subject *struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		HTMLURL string `json:"html_url"`
		Type    string `json:"type"`
		State   string `json:"state"`
	} `json:"subject"`
	Repository *struct {
		FullName string `json:"full_name"`
		HTMLURL  string `json:"html_url"`
	} `json:"repository"`
	UpdatedAt string `json:"updated_at"`
}

func mapNotifications(raw []byte) ([]signals.Item, time.Time, error) {
	var ns []notification
	if err := json.Unmarshal(raw, &ns); err != nil {
		return nil, time.Time{}, errs.Wrap(errs.KindSignal, err, "gitea: decoding notifications response")
	}
	items := make([]signals.Item, 0, len(ns))
	var newest time.Time
	for _, n := range ns {
		ts := parseTime(n.UpdatedAt)
		if ts.After(newest) {
			newest = ts
		}
		meta := map[string]string{"id": n.ID.String(), "updated": n.UpdatedAt}
		title, kind := "", "notification"
		if n.Subject != nil {
			title = n.Subject.Title
			kind = subjectKind(n.Subject.Type)
			if n.Subject.State != "" {
				meta["state"] = strings.ToLower(n.Subject.State)
			}
			if n.Subject.Type != "" {
				meta["subject_type"] = n.Subject.Type
			}
			if n.Subject.URL != "" {
				meta["api_url"] = n.Subject.URL
			}
		}
		items = append(items, signals.Item{
			Kind:      kind,
			Title:     title,
			Subtitle:  notificationRepo(n),
			URL:       browseURL(n),
			Timestamp: ts,
			Meta:      meta,
		})
	}
	return items, newest, nil
}

func subjectKind(subject string) string {
	switch strings.ToLower(subject) {
	case "pull":
		return "pr"
	case "issue":
		return "issue"
	case "commit":
		return "commit"
	case "repository":
		return "repo"
	}
	return "notification"
}

func notificationRepo(n notification) string {
	if n.Repository == nil {
		return ""
	}
	return n.Repository.FullName
}

func browseURL(n notification) string {
	if n.Subject == nil {
		if n.Repository != nil {
			return n.Repository.HTMLURL
		}
		return ""
	}
	if n.Subject.HTMLURL != "" {
		return n.Subject.HTMLURL
	}
	if web := webURLFromAPI(n.Subject.URL, subjectSegment(n.Subject.Type)); web != "" {
		return web
	}
	if n.Repository != nil && n.Repository.HTMLURL != "" {
		return n.Repository.HTMLURL
	}
	return n.Subject.URL
}

func subjectSegment(subject string) string {
	if strings.EqualFold(subject, "pull") {
		return "pulls"
	}
	return "issues"
}

func webURLFromAPI(rawAPI, segment string) string {
	const marker = "/api/v1/repos/"
	base, rest, ok := strings.Cut(rawAPI, marker)
	if !ok || base == "" {
		return ""
	}
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) < 4 {
		return ""
	}
	owner, repo, index := parts[0], parts[1], parts[len(parts)-1]
	if owner == "" || repo == "" || index == "" {
		return ""
	}
	return base + "/" + owner + "/" + repo + "/" + segment + "/" + index
}
