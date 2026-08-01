package github

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/errs"
)

func TestCheckGitHubStatusClassifiesRateLimits(t *testing.T) {
	reset := strconv.FormatInt(time.Now().Add(2*time.Minute).Unix(), 10)
	cases := []struct {
		name    string
		status  int
		header  http.Header
		body    string
		kind    errs.Kind
		hintHas string
		hintNot string
	}{
		{
			name:    "secondary rate limit",
			status:  http.StatusForbidden,
			header:  http.Header{"Retry-After": []string{"60"}},
			body:    `{"message":"You have exceeded a secondary rate limit. Please wait a few minutes before you try again."}`,
			kind:    errs.KindSignal,
			hintHas: "retry",
			hintNot: "mino login github",
		},
		{
			name:   "primary rate limit",
			status: http.StatusForbidden,
			header: http.Header{
				"X-Ratelimit-Remaining": []string{"0"},
				"X-Ratelimit-Reset":     []string{reset},
			},
			body:    `{"message":"API rate limit exceeded for user ID 1."}`,
			kind:    errs.KindSignal,
			hintHas: "retry",
			hintNot: "mino login github",
		},
		{
			name:    "too many requests",
			status:  http.StatusTooManyRequests,
			header:  http.Header{"Retry-After": []string{"30"}},
			body:    `{"message":"Too many requests"}`,
			kind:    errs.KindSignal,
			hintHas: "retry",
			hintNot: "mino login github",
		},
		{
			name:    "missing scope",
			status:  http.StatusForbidden,
			header:  http.Header{"X-Ratelimit-Remaining": []string{"4999"}},
			body:    `{"message":"Resource not accessible by personal access token"}`,
			kind:    errs.KindAuth,
			hintHas: "mino login github",
		},
		{
			name:   "missing scope whose doc link mentions rate limits",
			status: http.StatusForbidden,
			header: http.Header{"X-Ratelimit-Remaining": []string{"4998"}},
			body: `{"message":"Resource not accessible by personal access token",` +
				`"documentation_url":"https://docs.github.com/rest/overview/resources#rate limit policy"}`,
			kind:    errs.KindAuth,
			hintHas: "mino login github",
			hintNot: "rate limit reached",
		},
		{
			name:   "saml enforcement with an exhausted quota",
			status: http.StatusForbidden,
			header: http.Header{
				"X-Ratelimit-Remaining": []string{"0"},
				"X-Ratelimit-Reset":     []string{reset},
			},
			body: `{"message":"Resource protected by organization SAML enforcement. ` +
				`You must grant your OAuth token access to this organization."}`,
			kind:    errs.KindAuth,
			hintHas: "SAML",
		},
		{
			name:   "ip allow list",
			status: http.StatusForbidden,
			header: http.Header{"X-Ratelimit-Remaining": []string{"4999"}},
			body: `{"message":"Although you appear to have the correct authorization credentials, ` +
				`the ` + "`acme`" + ` organization has an IP allow list enabled."}`,
			kind:    errs.KindAuth,
			hintHas: "IP allow list",
			hintNot: "mino login github",
		},
		{
			name:    "secondary rate limit with no rate limit headers",
			status:  http.StatusForbidden,
			header:  http.Header{},
			body:    `{"message":"You have exceeded a secondary rate limit. Please wait a few minutes before you try again."}`,
			kind:    errs.KindSignal,
			hintHas: "rate limit",
			hintNot: "mino login github",
		},
		{
			name:    "bad credentials",
			status:  http.StatusUnauthorized,
			body:    `{"message":"Bad credentials"}`,
			kind:    errs.KindAuth,
			hintHas: "mino login github",
		},
		{
			name:    "saml enforcement",
			status:  http.StatusForbidden,
			header:  http.Header{"X-Ratelimit-Remaining": []string{"4999"}},
			body:    `{"message":"Resource protected by organization SAML enforcement. You must grant your OAuth token access to this organization."}`,
			kind:    errs.KindAuth,
			hintHas: "SAML",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			header := c.header
			if header == nil {
				header = http.Header{}
			}
			resp := &http.Response{StatusCode: c.status, Status: strconv.Itoa(c.status) + " x", Header: header}
			err := checkGitHubStatus(resp, []byte(c.body), "the notifications scope")
			if err == nil {
				t.Fatal("want an error")
			}
			var e *errs.Error
			if !errors.As(err, &e) {
				t.Fatalf("error %v is not an *errs.Error", err)
			}
			if e.Kind != c.kind {
				t.Errorf("kind = %q, want %q (hint %q)", e.Kind, c.kind, e.Hint)
			}
			if c.hintHas != "" && !strings.Contains(strings.ToLower(e.Hint), strings.ToLower(c.hintHas)) {
				t.Errorf("hint = %q, want it to mention %q", e.Hint, c.hintHas)
			}
			if c.hintNot != "" && strings.Contains(e.Hint, c.hintNot) {
				t.Errorf("hint = %q, should not mention %q", e.Hint, c.hintNot)
			}
		})
	}
}

func TestRateLimitedTrustsHeadersOverBodyProse(t *testing.T) {
	cases := []struct {
		name   string
		status int
		header http.Header
		body   string
		want   bool
	}{
		{
			name:   "429 is always a rate limit",
			status: http.StatusTooManyRequests,
			header: http.Header{},
			body:   `{"message":"Resource not accessible by personal access token"}`,
			want:   true,
		},
		{
			name:   "403 with an exhausted quota",
			status: http.StatusForbidden,
			header: http.Header{"X-Ratelimit-Remaining": []string{"0"}},
			body:   `{"message":"anything"}`,
			want:   true,
		},
		{
			name:   "403 with a retry-after",
			status: http.StatusForbidden,
			header: http.Header{"Retry-After": []string{"60"}},
			body:   `{"message":"anything"}`,
			want:   true,
		},
		{
			name:   "403 with quota left is not a rate limit however the body reads",
			status: http.StatusForbidden,
			header: http.Header{"X-Ratelimit-Remaining": []string{"4998"}},
			body: `{"message":"Resource not accessible by personal access token",` +
				`"documentation_url":"https://docs.github.com/rest#rate limit policy"}`,
			want: false,
		},
		{
			name:   "403 with no rate limit headers falls back to the body",
			status: http.StatusForbidden,
			header: http.Header{},
			body:   `{"message":"You have exceeded a secondary rate limit."}`,
			want:   true,
		},
		{
			name:   "403 with no rate limit headers and no prose",
			status: http.StatusForbidden,
			header: http.Header{},
			body:   `{"message":"Resource not accessible by integration"}`,
			want:   false,
		},
		{
			name:   "other statuses are never rate limits",
			status: http.StatusNotFound,
			header: http.Header{},
			body:   `{"message":"API rate limit exceeded"}`,
			want:   false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: c.status, Header: c.header}
			if got := rateLimited(resp, []byte(c.body)); got != c.want {
				t.Errorf("rateLimited = %v, want %v (headers are authoritative; body prose only fills the gap)", got, c.want)
			}
		})
	}
}

func TestPollIntervalHintIsBounded(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want time.Duration
	}{
		{name: "absent", raw: ""},
		{name: "zero", raw: "0"},
		{name: "negative", raw: "-30"},
		{name: "garbage", raw: "soon"},
		{name: "normal", raw: "60", want: time.Minute},
		{name: "padded", raw: " 90 ", want: 90 * time.Second},
		{name: "at the bound", raw: "3600", want: maxRetryAfter},
		{name: "absurd is capped", raw: "999999999", want: maxRetryAfter},
		{name: "overflowing is capped", raw: "99999999999999", want: maxRetryAfter},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hdr := http.Header{}
			hdr.Set("X-Poll-Interval", c.raw)
			if got := pollIntervalHint(hdr); got != c.want {
				t.Errorf("pollIntervalHint(%q) = %s, want %s", c.raw, got, c.want)
			}
		})
	}
}

func TestRetryAfterParsing(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		header http.Header
		want   time.Duration
		ok     bool
	}{
		{name: "no headers", header: http.Header{}},
		{name: "seconds", header: http.Header{"Retry-After": []string{"60"}}, want: time.Minute, ok: true},
		{name: "padded seconds", header: http.Header{"Retry-After": []string{" 30 "}}, want: 30 * time.Second, ok: true},
		{name: "zero seconds", header: http.Header{"Retry-After": []string{"0"}}},
		{
			name:   "http date",
			header: http.Header{"Retry-After": []string{now.Add(2 * time.Minute).Format(http.TimeFormat)}},
			want:   2 * time.Minute,
			ok:     true,
		},
		{name: "past date", header: http.Header{"Retry-After": []string{now.Add(-time.Minute).Format(http.TimeFormat)}}},
		{
			name: "ratelimit reset",
			header: http.Header{
				"X-Ratelimit-Remaining": []string{"0"},
				"X-Ratelimit-Reset":     []string{strconv.FormatInt(now.Add(5*time.Minute).Unix(), 10)},
			},
			want: 5 * time.Minute,
			ok:   true,
		},
		{
			name: "quota left",
			header: http.Header{
				"X-Ratelimit-Remaining": []string{"12"},
				"X-Ratelimit-Reset":     []string{strconv.FormatInt(now.Add(5*time.Minute).Unix(), 10)},
			},
		},
		{name: "absurd retry after is capped", header: http.Header{"Retry-After": []string{"999999"}}, want: maxRetryAfter, ok: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := retryAfter(c.header, now)
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v (got %s)", ok, c.ok, got)
			}
			if ok && got != c.want {
				t.Errorf("retryAfter = %s, want %s", got, c.want)
			}
		})
	}
}

func TestBackoffIntervalGrowsAndCaps(t *testing.T) {
	base := time.Minute
	cases := []struct {
		fails int
		want  time.Duration
	}{
		{fails: 0, want: base},
		{fails: 1, want: base},
		{fails: 2, want: 2 * time.Minute},
		{fails: 3, want: 4 * time.Minute},
		{fails: 4, want: 8 * time.Minute},
		{fails: 5, want: maxPollBackoff},
		{fails: 99, want: maxPollBackoff},
	}
	for _, c := range cases {
		if got := backoffInterval(base, c.fails); got != c.want {
			t.Errorf("backoffInterval(%s, %d) = %s, want %s", base, c.fails, got, c.want)
		}
	}
}

func TestWithJitterStaysInRange(t *testing.T) {
	const base = time.Minute
	seen := map[time.Duration]bool{}
	for range 200 {
		got := withJitter(base)
		if got < base || got > base+base/4 {
			t.Fatalf("withJitter(1m) = %s, want within [1m, 1m15s]", got)
		}
		seen[got] = true
	}
	if len(seen) < 10 {
		t.Errorf("withJitter(1m) produced %d distinct values over 200 draws, want a spread: a constant "+
			"result is no jitter at all, so every client retries in lockstep", len(seen))
	}
	if got := withJitter(0); got != 0 {
		t.Errorf("withJitter(0) = %s, want 0", got)
	}
	if got := withJitter(-time.Second); got != -time.Second {
		t.Errorf("withJitter(-1s) = %s, want it passed through", got)
	}
}

func TestRetryIntervalBacksOffOnRepeatedFailures(t *testing.T) {
	h := &activeSignal{interval: time.Minute}
	first := h.retryInterval(h.interval, 1)
	later := h.retryInterval(h.interval, 4)
	if first < time.Minute {
		t.Errorf("first retry = %s, want at least the poll interval", first)
	}
	if later < 8*time.Minute {
		t.Errorf("fourth retry = %s, want at least 8m of exponential backoff", later)
	}
	if honoured := h.retryInterval(30*time.Minute, 1); honoured < 30*time.Minute {
		t.Errorf("retryInterval = %s, want the server hint of 30m to win", honoured)
	}
}
