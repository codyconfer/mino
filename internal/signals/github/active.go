package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/codyconfer/mino/internal/auth"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/signals"
	"github.com/codyconfer/mino/internal/signals/active"
)

const (
	notificationsPath   = "/notifications?all=false&per_page=50"
	notificationMaxPage = 5
)

type activeSignal struct {
	src      auth.GitHubSource
	baseURL  string
	interval time.Duration
	http     *http.Client
	state    *active.State
}

func NewActive(src auth.GitHubSource, baseURL string, interval time.Duration, state *active.State) signals.ActiveSignal {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &activeSignal{src: src, baseURL: baseURL, interval: interval, http: signals.HTTPClient(), state: state}
}

func (h *activeSignal) Name() string { return "github" }

func (h *activeSignal) LatencyFloor() time.Duration { return h.interval }

func (h *activeSignal) client() *http.Client {
	if h.http == nil {
		return signals.HTTPClient()
	}
	return h.http
}

func (h *activeSignal) Stream(ctx context.Context) (<-chan signals.Event, error) {
	return active.PollAdaptive(ctx, "github", h.interval, h.step(ctx)), nil
}

func (h *activeSignal) step(ctx context.Context) func(context.Context) ([]signals.Item, time.Duration, error) {
	cursor := h.state.Cursor("github", "last_modified")
	seen := h.state.Seen("github:notifications")
	lastModified := cursor.Load(ctx)
	fails := 0
	return func(ctx context.Context) ([]signals.Item, time.Duration, error) {
		res, err := h.poll(ctx, lastModified)
		if err != nil {
			fails++
			return nil, h.retryInterval(res.next, fails), err
		}
		fails = 0
		if res.lastModified != "" && res.lastModified != lastModified {
			lastModified = res.lastModified
			_ = cursor.Save(ctx, res.lastModified)
		}
		if res.notModified {
			return nil, res.next, nil
		}
		return seen.Unseen(ctx, res.items, notificationKey), res.next, nil
	}
}

func notificationKey(it signals.Item) string {
	return it.Meta["id"] + "|" + it.Meta["updated"]
}

type pollResult struct {
	items        []signals.Item
	next         time.Duration
	lastModified string
	notModified  bool
}

func partial(res pollResult) pollResult {
	res.lastModified = ""
	res.notModified = false
	return res
}

func (h *activeSignal) poll(ctx context.Context, lastModified string) (pollResult, error) {
	res := pollResult{next: h.interval}
	pageURL := githubBaseURL(h.baseURL) + notificationsPath
	for page := 0; pageURL != ""; page++ {
		if page >= notificationMaxPage {
			log.Debugf("github: stopping after %d notification page(s); %d thread(s) collected and more remain",
				notificationMaxPage, len(res.items))
			break
		}
		since := ""
		if page == 0 {
			since = lastModified
		}
		pg, err := h.fetchPage(ctx, pageURL, since)
		if err != nil {
			if len(res.items) > 0 {
				log.Debugf("github: notifications page %d failed after %d thread(s); discarding the poll: %v",
					page+1, len(res.items), err)
				res.next = max(res.next, pg.next)
			} else {
				res.next = pg.next
			}
			return partial(res), err
		}
		if page == 0 {
			res.next = pg.next
			res.lastModified = pg.lastModified
			if pg.notModified {
				res.notModified = true
				return res, nil
			}
		}
		items, err := mapNotifications(pg.body)
		if err != nil {
			if len(res.items) > 0 {
				log.Debugf("github: notifications page %d unreadable after %d thread(s); discarding the poll: %v",
					page+1, len(res.items), err)
			}
			return partial(res), err
		}
		res.items = append(res.items, items...)
		pageURL = pg.nextURL
	}
	return res, nil
}

type notificationPage struct {
	body         []byte
	nextURL      string
	next         time.Duration
	lastModified string
	notModified  bool
}

func (h *activeSignal) fetchPage(ctx context.Context, rawURL, since string) (notificationPage, error) {
	pg := notificationPage{next: h.interval}
	tok, err := h.src.Token(ctx)
	if err != nil {
		return pg, err
	}
	req, err := newGitHubURLRequest(ctx, rawURL, tok)
	if err != nil {
		return pg, errs.Wrap(errs.KindSignal, err, "github: building notifications request")
	}
	if since != "" {
		req.Header.Set("If-Modified-Since", since)
	}
	resp, err := h.client().Do(req)
	if err != nil {
		return pg, errs.Wrap(errs.KindSignal, err, "github: notifications request failed")
	}
	defer resp.Body.Close()
	pg.next = h.nextInterval(resp)
	pg.lastModified = resp.Header.Get("Last-Modified")
	if resp.StatusCode == http.StatusNotModified {
		pg.notModified = true
		return pg, nil
	}
	body, err := readBody(resp)
	if err != nil {
		return pg, err
	}
	if err := checkGitHubStatus(resp, body, "the notifications scope"); err != nil {
		return pg, err
	}
	pg.body = body
	pg.nextURL = nextPageURL(resp.Header.Get("Link"), rawURL)
	return pg, nil
}

func (h *activeSignal) nextInterval(resp *http.Response) time.Duration {
	next := h.interval
	if hint := pollIntervalHint(resp.Header); hint > next {
		next = hint
	}
	if d, ok := retryAfter(resp.Header, time.Now()); ok {
		if d = withJitter(d); d > next {
			next = d
		}
	}
	return next
}

func (h *activeSignal) retryInterval(next time.Duration, fails int) time.Duration {
	return max(next, withJitter(backoffInterval(h.interval, fails)))
}

func nextPageURL(link, current string) string {
	if strings.TrimSpace(link) == "" {
		return ""
	}
	cur, err := url.Parse(current)
	if err != nil {
		return ""
	}
	for _, part := range strings.Split(link, ",") {
		fields := strings.Split(strings.TrimSpace(part), ";")
		if len(fields) < 2 {
			continue
		}
		target := strings.TrimSpace(fields[0])
		if !strings.HasPrefix(target, "<") || !strings.HasSuffix(target, ">") {
			continue
		}
		if !hasRel(fields[1:], "next") {
			continue
		}
		u, err := url.Parse(strings.Trim(target, "<>"))
		if err != nil || u.Scheme != cur.Scheme || u.Host != cur.Host {
			continue
		}
		return u.String()
	}
	return ""
}

func hasRel(params []string, want string) bool {
	for _, p := range params {
		k, v, ok := strings.Cut(strings.TrimSpace(p), "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(k), "rel") {
			continue
		}
		if strings.Trim(strings.TrimSpace(v), `"`) == want {
			return true
		}
	}
	return false
}

type notification struct {
	ID      string `json:"id"`
	Reason  string `json:"reason"`
	Subject struct {
		Title string `json:"title"`
		URL   string `json:"url"`
	} `json:"subject"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	UpdatedAt string `json:"updated_at"`
}

func mapNotifications(raw []byte) ([]signals.Item, error) {
	var ns []notification
	if err := json.Unmarshal(raw, &ns); err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "github: decoding notifications response")
	}
	items := make([]signals.Item, 0, len(ns))
	for _, n := range ns {
		var ts time.Time
		if n.UpdatedAt != "" {
			ts, _ = time.Parse(time.RFC3339, n.UpdatedAt)
		}
		items = append(items, signals.Item{
			Kind:      n.Reason,
			Title:     n.Subject.Title,
			Subtitle:  n.Repository.FullName,
			URL:       n.Subject.URL,
			Timestamp: ts,
			Meta:      map[string]string{"id": n.ID, "updated": n.UpdatedAt},
		})
	}
	return items, nil
}
