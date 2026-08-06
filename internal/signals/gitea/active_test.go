package gitea

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/auth"
	"github.com/codyconfer/mino/internal/signals"
	"github.com/codyconfer/mino/internal/signals/active"
)

func threadJSON(id int, updated string) string {
	return fmt.Sprintf(`{"id":%d,"unread":true,"updated_at":%q,`+
		`"subject":{"title":"thread %d","url":"https://git.example.com/api/v1/repos/acme/tools/issues/7","type":"Pull","state":"open"},`+
		`"repository":{"full_name":"acme/tools","html_url":"https://git.example.com/acme/tools"}}`, id, updated, id)
}

func threadsJSON(ids ...int) string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, threadJSON(id, "2026-07-20T15:04:05Z"))
	}
	return "[" + strings.Join(out, ",") + "]"
}

func fullPage() []int {
	ids := make([]int, 0, notificationLimit)
	for i := 1; i <= notificationLimit; i++ {
		ids = append(ids, i)
	}
	return ids
}

type notifyRequest struct {
	since       string
	page        int
	limit       string
	statusTypes string
}

type notifyServer struct {
	*httptest.Server

	mu       sync.Mutex
	requests []notifyRequest
	polls    int
}

func newNotifyServer(t *testing.T, handler func(poll, page int) (int, string, http.Header)) *notifyServer {
	t.Helper()
	n := &notifyServer{}
	n.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		page, _ := strconv.Atoi(q.Get("page"))
		n.mu.Lock()
		if page <= 1 {
			n.polls++
		}
		poll := n.polls
		n.requests = append(n.requests, notifyRequest{
			since:       q.Get("since"),
			page:        page,
			limit:       q.Get("limit"),
			statusTypes: q.Get("status-types"),
		})
		n.mu.Unlock()

		code, payload, hdr := handler(poll, page)
		for k, vals := range hdr {
			w.Header().Set(k, vals[0])
		}
		w.WriteHeader(code)
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(n.Close)
	return n
}

func (n *notifyServer) seen() []notifyRequest {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]notifyRequest(nil), n.requests...)
}

func (n *notifyServer) waitForRequests(t *testing.T, want int) []notifyRequest {
	t.Helper()
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if seen := n.seen(); len(seen) >= want {
			return seen
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("only %d of %d request(s) reached the server", len(n.seen()), want)
	return nil
}

func startStream(t *testing.T, srv *notifyServer, interval time.Duration) <-chan signals.Event {
	t.Helper()
	sig := NewActive(auth.StaticGiteaToken("tok"), srv.URL+"/api/v1", interval, active.NewState(nil))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	events, err := sig.Stream(ctx)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	return events
}

func TestStreamSendsSinceOnlyOnceACursorExists(t *testing.T) {
	srv := newNotifyServer(t, func(int, int) (int, string, http.Header) {
		return http.StatusOK, threadsJSON(1), nil
	})
	startStream(t, srv, 40*time.Millisecond)

	seen := srv.waitForRequests(t, 2)
	if seen[0].since != "" {
		t.Errorf("first request sent since=%q, want none: there is no cursor yet", seen[0].since)
	}
	if seen[1].since != "2026-07-20T15:04:05Z" {
		t.Errorf("second request sent since=%q, want the newest updated_at; gitea sends no Last-Modified to use instead", seen[1].since)
	}
	if seen[0].limit != strconv.Itoa(notificationLimit) || seen[0].statusTypes != "unread" || seen[0].page != 1 {
		t.Errorf("query = %+v, want the unread page-1 read", seen[0])
	}
}

func TestStreamEmitsEachThreadOnce(t *testing.T) {
	srv := newNotifyServer(t, func(poll, _ int) (int, string, http.Header) {
		if poll >= 3 {
			return http.StatusOK, threadsJSON(1, 2), nil
		}
		return http.StatusOK, threadsJSON(1), nil
	})
	events := startStream(t, srv, 30*time.Millisecond)

	select {
	case ev := <-events:
		if ev.Section.Err != nil {
			t.Fatalf("stream error: %v", ev.Section.Err)
		}
		if len(ev.Section.Items) != 1 {
			t.Fatalf("delivered %d items, want only the thread that is new", len(ev.Section.Items))
		}
		if got := ev.Section.Items[0].Meta["id"]; got != "2" {
			t.Errorf("delivered thread %q, want 2: gitea ids are numbers and the first poll is a baseline", got)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("no event for a thread that appeared after the baseline poll")
	}
}

func TestStreamStopsPagingOnAShortPage(t *testing.T) {
	srv := newNotifyServer(t, func(_, page int) (int, string, http.Header) {
		if page <= 1 {
			return http.StatusOK, threadsJSON(fullPage()...), nil
		}
		return http.StatusOK, threadsJSON(999), nil
	})
	startStream(t, srv, time.Hour)

	seen := srv.waitForRequests(t, 2)
	time.Sleep(100 * time.Millisecond)
	if got := len(srv.seen()); got != 2 {
		t.Errorf("made %d requests, want 2: a page shorter than the limit is the last one", got)
	}
	if seen[1].page != 2 {
		t.Errorf("second request read page %d, want 2", seen[1].page)
	}
}

func TestStreamCapsPaging(t *testing.T) {
	srv := newNotifyServer(t, func(int, int) (int, string, http.Header) {
		return http.StatusOK, threadsJSON(fullPage()...), nil
	})
	startStream(t, srv, time.Hour)

	srv.waitForRequests(t, notificationMaxPage)
	time.Sleep(100 * time.Millisecond)
	if got := len(srv.seen()); got != notificationMaxPage {
		t.Errorf("made %d requests, want the %d-page cap", got, notificationMaxPage)
	}
}

func TestStreamKeepsTheCursorWhenAPageFails(t *testing.T) {
	srv := newNotifyServer(t, func(_, page int) (int, string, http.Header) {
		if page <= 1 {
			return http.StatusOK, threadsJSON(fullPage()...), nil
		}
		return http.StatusInternalServerError, `{"message":"boom"}`, nil
	})
	events := startStream(t, srv, 40*time.Millisecond)

	select {
	case ev := <-events:
		if ev.Section.Err == nil {
			t.Fatal("a failed second page was reported as a successful poll")
		}
		if len(ev.Section.Items) != 0 {
			t.Errorf("delivered %d items from a discarded poll", len(ev.Section.Items))
		}
	case <-time.After(6 * time.Second):
		t.Fatal("a failing page produced no error event")
	}

	for _, req := range srv.waitForRequests(t, 3) {
		if req.since != "" {
			t.Fatalf("a discarded poll advanced the cursor to %q; the threads it never read would be skipped", req.since)
		}
	}
}

func TestStreamReportsAnUnreachableInstance(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)

	sig := NewActive(auth.StaticGiteaToken("tok"), srv.URL+"/api/v1", 300*time.Millisecond, active.NewState(nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events, err := sig.Stream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-events:
		if ev.Section.Err == nil {
			t.Fatalf("want an error event from a server that never responds, got %#v", ev.Section)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("no event within 8s: the poll step hung on a server that never responds")
	}
}

func TestNextPollIntervalHonoursRetryAfterOnly(t *testing.T) {
	base := time.Minute
	resp := &http.Response{Header: http.Header{"Retry-After": {"120"}}}
	if got := nextPollInterval(base, resp); got < 2*time.Minute || got > 150*time.Second {
		t.Errorf("nextPollInterval = %s, want about 2m with jitter", got)
	}

	resp = &http.Response{Header: http.Header{"X-Poll-Interval": {"600"}}}
	if got := nextPollInterval(base, resp); got != base {
		t.Errorf("nextPollInterval = %s, want the base interval: gitea does not send X-Poll-Interval", got)
	}
}

func TestMapNotificationsBuildsBrowsableItems(t *testing.T) {
	items, newest, err := mapNotifications([]byte(threadsJSON(1, 2)))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	it := items[0]
	if it.Kind != "pr" {
		t.Errorf("kind = %q, want pr from subject.type", it.Kind)
	}
	if it.Subtitle != "acme/tools" {
		t.Errorf("subtitle = %q, want the repo slug", it.Subtitle)
	}
	if it.URL != "https://git.example.com/acme/tools/pulls/7" {
		t.Errorf("URL = %q, want the API url rewritten to a browsable one", it.URL)
	}
	if it.Meta["api_url"] == "" || it.Meta["state"] != "open" {
		t.Errorf("meta = %v, want the api url and state carried", it.Meta)
	}
	if it.Meta["id"] != "1" {
		t.Errorf("meta id = %q, want 1 formatted from a JSON number", it.Meta["id"])
	}
	if newest.Format(time.RFC3339) != "2026-07-20T15:04:05Z" {
		t.Errorf("newest = %s, want the max updated_at, which becomes the cursor", newest)
	}
}

func TestBrowseURLPrefersHTMLURL(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "html_url wins",
			raw: `{"id":1,"subject":{"title":"t","type":"Pull","url":"https://git.example.com/api/v1/repos/acme/tools/issues/7",` +
				`"html_url":"https://git.example.com/acme/tools/pulls/7"},"repository":{"full_name":"acme/tools"}}`,
			want: "https://git.example.com/acme/tools/pulls/7",
		},
		{
			name: "issue subject maps to issues",
			raw:  `{"id":1,"subject":{"title":"t","type":"Issue","url":"https://git.example.com/api/v1/repos/acme/tools/issues/9"}}`,
			want: "https://git.example.com/acme/tools/issues/9",
		},
		{
			name: "subpath install is preserved",
			raw:  `{"id":1,"subject":{"title":"t","type":"Pull","url":"https://example.com/gitea/api/v1/repos/acme/tools/issues/7"}}`,
			want: "https://example.com/gitea/acme/tools/pulls/7",
		},
		{
			name: "unparsable falls back to the repo",
			raw:  `{"id":1,"subject":{"title":"t","type":"Commit","url":"https://git.example.com/weird"},"repository":{"html_url":"https://git.example.com/acme/tools"}}`,
			want: "https://git.example.com/acme/tools",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			items, _, err := mapNotifications([]byte("[" + c.raw + "]"))
			if err != nil {
				t.Fatal(err)
			}
			if items[0].URL != c.want {
				t.Errorf("URL = %q, want %q", items[0].URL, c.want)
			}
		})
	}
}
