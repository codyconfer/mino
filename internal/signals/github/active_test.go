package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/signals"
	"github.com/codyconfer/munin/internal/signals/active"
)

func notificationsJSON(ids ...string) string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, `{"id":"`+id+`","reason":"review_requested",`+
			`"subject":{"title":"thread `+id+`","url":"https://api.github.com/repos/o/r/pulls/1"},`+
			`"repository":{"full_name":"o/r"},"updated_at":"2026-07-20T15:04:05Z"}`)
	}
	return "[" + strings.Join(out, ",") + "]"
}

func TestActiveStreamBoundsHangingPoll(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)

	sig := NewActive("tok", srv.URL, 300*time.Millisecond, active.NewState(nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events, err := sig.Stream(ctx)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	select {
	case ev := <-events:
		if ev.Section.Err == nil {
			t.Fatalf("want an error event from a server that never responds, got %#v", ev.Section)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no event within 3s: the poll step hung on a server that never responds")
	}
}

func TestActiveStreamFollowsNotificationPages(t *testing.T) {
	var (
		mu       sync.Mutex
		polls    int
		perPages []string
		srv      *httptest.Server
	)
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		page := r.URL.Query().Get("page")
		perPages = append(perPages, r.URL.Query().Get("per_page"))
		if page == "" || page == "1" {
			polls++
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case polls == 1:
			_, _ = w.Write([]byte(`[]`))
		case page == "2":
			_, _ = w.Write([]byte(notificationsJSON("n4")))
		default:
			w.Header().Set("Link", `<`+srv.URL+`/notifications?all=false&per_page=50&page=2>; rel="next"`)
			_, _ = w.Write([]byte(notificationsJSON("n2", "n3")))
		}
	}))
	defer srv.Close()

	sig := NewActive("tok", srv.URL, 150*time.Millisecond, active.NewState(nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events, err := sig.Stream(ctx)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	select {
	case ev := <-events:
		if ev.Section.Err != nil {
			t.Fatalf("event error: %v", ev.Section.Err)
		}
		if len(ev.Section.Items) != 3 {
			ids := make([]string, 0, len(ev.Section.Items))
			for _, it := range ev.Section.Items {
				ids = append(ids, it.Meta["id"])
			}
			t.Fatalf("got %d items %v, want 3 (the rel=\"next\" page was dropped)", len(ev.Section.Items), ids)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no notification event within 5s")
	}
	mu.Lock()
	defer mu.Unlock()
	for _, pp := range perPages {
		if pp != "50" {
			t.Errorf("per_page = %q, want 50", pp)
		}
	}
}

func TestNextIntervalHonoursRetryAfter(t *testing.T) {
	h := &activeSignal{interval: 10 * time.Second}
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Header: http.Header{
			"Retry-After": []string{"60"},
		},
	}
	got := h.nextInterval(resp)
	if got < 60*time.Second {
		t.Fatalf("nextInterval = %s, want at least the 60s Retry-After", got)
	}
	if got > 90*time.Second {
		t.Fatalf("nextInterval = %s, want no more than 60s plus jitter", got)
	}
}

func TestNextIntervalHonoursRateLimitReset(t *testing.T) {
	reset := time.Now().Add(90 * time.Second).Unix()
	h := &activeSignal{interval: 10 * time.Second}
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Header: http.Header{
			"X-Ratelimit-Remaining": []string{"0"},
			"X-Ratelimit-Reset":     []string{strconv.FormatInt(reset, 10)},
		},
	}
	got := h.nextInterval(resp)
	if got < 80*time.Second {
		t.Fatalf("nextInterval = %s, want roughly the 90s until x-ratelimit-reset", got)
	}
	if got > 2*time.Minute {
		t.Fatalf("nextInterval = %s, want no more than the reset plus jitter", got)
	}
}

func TestNextIntervalKeepsPollIntervalHint(t *testing.T) {
	h := &activeSignal{interval: 10 * time.Second}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Poll-Interval": []string{"60"}},
	}
	if got := h.nextInterval(resp); got != 60*time.Second {
		t.Fatalf("nextInterval = %s, want 60s", got)
	}
	if got := h.nextInterval(&http.Response{StatusCode: http.StatusOK, Header: http.Header{}}); got != 10*time.Second {
		t.Fatalf("nextInterval = %s, want the configured 10s", got)
	}
}

func TestNewActiveUsesSharedClient(t *testing.T) {
	h, ok := NewActive("tok", "", time.Minute, nil).(*activeSignal)
	if !ok {
		t.Fatal("NewActive did not return an *activeSignal")
	}
	if h.http != signals.HTTPClient() {
		t.Error("the active signal must use the shared client so it inherits a request timeout")
	}
	if h.client().Timeout <= 0 {
		t.Error("the active signal client has no timeout")
	}
	var zero activeSignal
	if zero.client() != signals.HTTPClient() {
		t.Error("a zero-value activeSignal must fall back to the shared client")
	}
}

func TestActiveStreamRateLimitSurfacesSignalError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"You have exceeded a secondary rate limit. Please wait a few minutes before you try again."}`))
	}))
	defer srv.Close()

	sig := NewActive("tok", srv.URL, 200*time.Millisecond, active.NewState(nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events, err := sig.Stream(ctx)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	select {
	case ev := <-events:
		if ev.Section.Err == nil {
			t.Fatal("want a rate-limit error event")
		}
		var e *errs.Error
		if !errors.As(ev.Section.Err, &e) {
			t.Fatalf("error %v is not an *errs.Error", ev.Section.Err)
		}
		if e.Kind != errs.KindSignal {
			t.Errorf("kind = %q, want %q (a rate limit is not an auth problem)", e.Kind, errs.KindSignal)
		}
		if !strings.Contains(strings.ToLower(e.Hint), "rate limit") {
			t.Errorf("hint = %q, want it to mention the rate limit", e.Hint)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no event within 3s")
	}
}

func TestActiveStreamStopsAtNotificationPageCap(t *testing.T) {
	var (
		mu       sync.Mutex
		requests int
		srv      *httptest.Server
	)
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		n := requests
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", `<`+srv.URL+`/notifications?page=`+strconv.Itoa(n+1)+`>; rel="next"`)
		_, _ = w.Write([]byte(notificationsJSON("n" + strconv.Itoa(n))))
	}))
	defer srv.Close()

	h := &activeSignal{token: "tok", baseURL: srv.URL, interval: time.Minute, http: srv.Client()}
	res, err := h.poll(context.Background(), "")
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(res.items) != notificationMaxPage {
		t.Errorf("collected %d items, want %d (one per page up to the cap)", len(res.items), notificationMaxPage)
	}
	mu.Lock()
	defer mu.Unlock()
	if requests != notificationMaxPage {
		t.Errorf("made %d requests, want at most %d", requests, notificationMaxPage)
	}
}

func TestActivePollKeepsPartialPagesOnLaterFailure(t *testing.T) {
	var (
		mu       sync.Mutex
		requests int
		srv      *httptest.Server
	)
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		requests++
		n := requests
		mu.Unlock()
		if n > 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"boom"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", `<`+srv.URL+`/notifications?page=2>; rel="next"`)
		_, _ = w.Write([]byte(notificationsJSON("n1", "n2")))
	}))
	defer srv.Close()

	h := &activeSignal{token: "tok", baseURL: srv.URL, interval: time.Minute, http: srv.Client()}
	res, err := h.poll(context.Background(), "")
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(res.items) != 2 {
		t.Errorf("collected %d items, want the 2 from the first page", len(res.items))
	}
}

func TestActivePollNotModified(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("If-Modified-Since")
		w.Header().Set("Last-Modified", "Thu, 30 Jul 2026 12:00:00 GMT")
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	h := &activeSignal{token: "tok", baseURL: srv.URL, interval: time.Minute, http: srv.Client()}
	res, err := h.poll(context.Background(), "Wed, 29 Jul 2026 12:00:00 GMT")
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if !res.notModified {
		t.Error("want notModified")
	}
	if got != "Wed, 29 Jul 2026 12:00:00 GMT" {
		t.Errorf("If-Modified-Since = %q", got)
	}
	if res.lastModified != "Thu, 30 Jul 2026 12:00:00 GMT" {
		t.Errorf("lastModified = %q", res.lastModified)
	}
}

func TestNextPageURL(t *testing.T) {
	const cur = "https://api.github.com/notifications?all=false&per_page=50"
	cases := []struct {
		name string
		link string
		want string
	}{
		{name: "empty", link: ""},
		{
			name: "next and last",
			link: `<https://api.github.com/notifications?page=2>; rel="next", <https://api.github.com/notifications?page=3>; rel="last"`,
			want: "https://api.github.com/notifications?page=2",
		},
		{
			name: "prev only",
			link: `<https://api.github.com/notifications?page=1>; rel="prev"`,
		},
		{
			name: "other host is ignored",
			link: `<https://evil.example.com/notifications?page=2>; rel="next"`,
		},
		{
			name: "other scheme is ignored",
			link: `<http://api.github.com/notifications?page=2>; rel="next"`,
		},
		{
			name: "unbracketed target is ignored",
			link: `https://api.github.com/notifications?page=2; rel="next"`,
		},
		{
			name: "unquoted rel",
			link: `<https://api.github.com/notifications?page=2>; rel=next`,
			want: "https://api.github.com/notifications?page=2",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := nextPageURL(c.link, cur); got != c.want {
				t.Errorf("nextPageURL = %q, want %q", got, c.want)
			}
		})
	}
}
