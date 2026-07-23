package github

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/codyconfer/munin/internal/active"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/signals"
)

type activeSignal struct {
	token    string
	baseURL  string
	interval time.Duration
	http     *http.Client
	state    *active.State
}

func NewActive(token, baseURL string, interval time.Duration, state *active.State) signals.ActiveSignal {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &activeSignal{token: token, baseURL: baseURL, interval: interval, http: http.DefaultClient, state: state}
}

func (h *activeSignal) Name() string { return "github" }

func (h *activeSignal) LatencyFloor() time.Duration { return h.interval }

func (h *activeSignal) Stream(ctx context.Context) (<-chan signals.Event, error) {
	cursor := h.state.Cursor("github", "last_modified")
	lastModified := cursor.Load()
	seen := h.state.Seen("github:notifications")
	step := func(ctx context.Context) ([]signals.Item, time.Duration, error) {
		base := strings.TrimRight(h.baseURL, "/")
		if base == "" {
			base = "https://api.github.com"
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/notifications?all=false", nil)
		if err != nil {
			return nil, 0, errs.Wrap(errs.KindSignal, err, "github: building notifications request")
		}
		req.Header.Set("Authorization", "Bearer "+h.token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		if lastModified != "" {
			req.Header.Set("If-Modified-Since", lastModified)
		}
		hc := h.http
		if hc == nil {
			hc = http.DefaultClient
		}
		resp, err := hc.Do(req)
		if err != nil {
			return nil, 0, errs.Wrap(errs.KindSignal, err, "github: notifications request failed")
		}
		defer resp.Body.Close()
		next := h.nextInterval(resp)
		if resp.StatusCode == http.StatusNotModified {
			return nil, next, nil
		}
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, next, errs.Newf(errs.KindAuth, "github api %s: %s", resp.Status, strings.TrimSpace(string(body))).
				WithHint("your GitHub token may be missing or lack the notifications scope; run `munin login github` or set $GITHUB_TOKEN")
		}
		if resp.StatusCode >= 400 {
			return nil, next, errs.Newf(errs.KindSignal, "github api %s: %s", resp.Status, strings.TrimSpace(string(body)))
		}
		if lm := resp.Header.Get("Last-Modified"); lm != "" && lm != lastModified {
			lastModified = lm
			_ = cursor.Save(lm)
		}
		items, err := mapNotifications(body)
		if err != nil {
			return nil, next, err
		}
		return seen.Fresh(items, func(it signals.Item) string { return it.Meta["id"] + "|" + it.Meta["updated"] }), next, nil
	}
	return active.PollAdaptive(ctx, "github", h.interval, step), nil
}

func (h *activeSignal) nextInterval(resp *http.Response) time.Duration {
	hint := resp.Header.Get("X-Poll-Interval")
	if hint == "" {
		return h.interval
	}
	secs, err := strconv.Atoi(strings.TrimSpace(hint))
	if err != nil || secs <= 0 {
		return h.interval
	}
	server := time.Duration(secs) * time.Second
	if server > h.interval {
		return server
	}
	return h.interval
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
