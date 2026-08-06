package gitlab

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/errs"
)

func TestCheckGitLabStatus(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		header   http.Header
		body     string
		scoped   bool
		wantKind errs.Kind
		wantHint string
	}{
		{
			name: "unauthorized", status: http.StatusUnauthorized,
			body: `{"message":"401 Unauthorized"}`, wantKind: errs.KindAuth,
			wantHint: "mino login gitlab",
		},
		{
			name: "insufficient scope names it", status: http.StatusForbidden,
			body: `{"error":"insufficient_scope","scope":"read_api"}`, wantKind: errs.KindAuth,
			wantHint: "read_api scope",
		},
		{
			name: "rate limited by header", status: http.StatusForbidden,
			header: http.Header{"Ratelimit-Remaining": {"0"}, "Ratelimit-Reset": {"1785931260"}},
			body:   `{"message":"429"}`, wantKind: errs.KindSignal, wantHint: "retry after",
		},
		{
			name: "too many requests", status: http.StatusTooManyRequests,
			body: `Retry later`, wantKind: errs.KindSignal, wantHint: "rate limit",
		},
		{
			name: "scoped not found", status: http.StatusNotFound, scoped: true,
			body: `{"message":"404 Project Not Found"}`, wantKind: errs.KindUsage,
			wantHint: "cannot see",
		},
		{
			name: "unscoped not found", status: http.StatusNotFound,
			body: `{"message":"404 Not Found"}`, wantKind: errs.KindSignal,
		},
		{
			name: "plain forbidden", status: http.StatusForbidden,
			body: `{"message":"403 Forbidden"}`, wantKind: errs.KindAuth, wantHint: "glab auth login",
		},
		{
			name: "server error", status: http.StatusBadGateway,
			body: `oops`, wantKind: errs.KindSignal,
		},
		{name: "ok", status: http.StatusOK},
	}
	freezeClock(t, "2026-08-05T12:00:00Z")
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: c.status, Status: http.StatusText(c.status), Header: c.header}
			if resp.Header == nil {
				resp.Header = http.Header{}
			}
			err := checkGitLabStatus(resp, []byte(c.body), "read_api", c.scoped)
			if c.wantKind == "" {
				if err != nil {
					t.Fatalf("a 200 produced %v", err)
				}
				return
			}
			if errs.KindOf(err) != c.wantKind {
				t.Errorf("kind = %v, want %v (%v)", errs.KindOf(err), c.wantKind, err)
			}
			if c.wantHint != "" && !strings.Contains(errs.Hint(err), c.wantHint) {
				t.Errorf("hint = %q, want it to mention %q", errs.Hint(err), c.wantHint)
			}
		})
	}
}

func TestErrorBodiesAreSanitizedAndBounded(t *testing.T) {
	nasty := "\x1b[2J\x07" + strings.Repeat("noise ", 2000)
	got := gitlabMessage([]byte(`{"message":"` + nasty + `"}`))

	if strings.ContainsAny(got, "\x1b\x07") {
		t.Errorf("terminal control bytes reached the message: %q", got)
	}
	if strings.Contains(got, "\n") {
		t.Errorf("a multi-line message reached the renderer: %q", got)
	}
	if len(got) > 4096 {
		t.Errorf("message is %d bytes; a hostile body must not fill the screen", len(got))
	}
}

func TestOversizeBodyIsRefusedAndStillClassified(t *testing.T) {
	big := strings.NewReader(strings.Repeat("x", maxResponseBytes+1024))

	resp := &http.Response{
		StatusCode:    http.StatusUnauthorized,
		Status:        "401 Unauthorized",
		Header:        http.Header{},
		Body:          readCloser{big},
		ContentLength: -1,
	}
	_, err := readBody(resp)
	if err == nil {
		t.Fatal("a body over the limit was accepted")
	}
	if errs.KindOf(err) != errs.KindAuth {
		t.Errorf("kind = %v, want auth; the status still classifies even when the body is refused",
			errs.KindOf(err))
	}

	ok := &http.Response{
		StatusCode:    http.StatusOK,
		Status:        "200 OK",
		Header:        http.Header{},
		Body:          readCloser{strings.NewReader(strings.Repeat("x", maxResponseBytes+1024))},
		ContentLength: -1,
	}
	_, err = readBody(ok)
	if err == nil {
		t.Fatal("an oversize 200 was accepted")
	}
	if !strings.Contains(errs.Hint(err), "gitlab.api_url") {
		t.Errorf("hint = %q, want it to point at the endpoint setting", errs.Hint(err))
	}
}

type readCloser struct{ *strings.Reader }

func (readCloser) Close() error { return nil }

func TestRetryAfterReadsUnprefixedHeaders(t *testing.T) {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		header  http.Header
		wantSec float64
		wantOK  bool
	}{
		{"none", http.Header{}, 0, false},
		{"retry-after seconds", http.Header{"Retry-After": {"90"}}, 90, true},
		{
			"ratelimit reset",
			http.Header{"Ratelimit-Remaining": {"0"}, "Ratelimit-Reset": {"1785931260"}}, 60, true,
		},
		{
			"reset time header",
			http.Header{
				"Ratelimit-Remaining": {"0"},
				"Ratelimit-Resettime": {"Wed, 05 Aug 2026 12:02:00 GMT"},
			}, 120, true,
		},
		{
			"remaining above zero",
			http.Header{"Ratelimit-Remaining": {"5"}, "Ratelimit-Reset": {"1785931260"}}, 0, false,
		},
		{"past reset", http.Header{"Ratelimit-Remaining": {"0"}, "Ratelimit-Reset": {"1"}}, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, ok := retryAfter(c.header, now)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if ok && d.Seconds() != c.wantSec {
				t.Errorf("delay = %v, want %vs", d, c.wantSec)
			}
		})
	}
}

func TestRetryAfterIsBounded(t *testing.T) {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	if d, ok := retryAfter(http.Header{"Retry-After": {"999999"}}, now); !ok || d != maxRetryAfter {
		t.Errorf("delay = %v/%v, want it clamped to %v", d, ok, maxRetryAfter)
	}
}

func TestBackoffGrowsAndIsCapped(t *testing.T) {
	base := time.Minute
	if got := backoffInterval(base, 1); got != base {
		t.Errorf("one failure = %v, want the base interval", got)
	}
	if got := backoffInterval(base, 3); got != 4*time.Minute {
		t.Errorf("three failures = %v, want 4m", got)
	}
	if got := backoffInterval(base, 50); got != maxPollBackoff {
		t.Errorf("many failures = %v, want the %v cap", got, maxPollBackoff)
	}
}

func TestRateHintDecays(t *testing.T) {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	h := &RateHint{}
	if got := h.delay(now); got != 0 {
		t.Errorf("a fresh hint delays %v, want 0", got)
	}

	h.observe(http.Header{"Retry-After": {"120"}}, now)
	if got := h.delay(now.Add(30 * time.Second)); got != 90*time.Second {
		t.Errorf("delay = %v, want 90s remaining", got)
	}
	if got := h.delay(now.Add(5 * time.Minute)); got != 0 {
		t.Errorf("delay = %v after the window, want 0", got)
	}
}

func TestCLIErrorPassesThroughUnrelatedFailures(t *testing.T) {
	orig := errs.New(errs.KindSignal, "glab: exec format error")
	if got := cliError(orig); got != orig {
		t.Errorf("cliError rewrote an unrelated failure: %v", got)
	}
	if cliError(nil) != nil {
		t.Error("cliError(nil) is not nil")
	}
}
