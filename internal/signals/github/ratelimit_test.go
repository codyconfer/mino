package github

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/munin/internal/errs"
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
			hintNot: "munin login github",
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
			hintNot: "munin login github",
		},
		{
			name:    "too many requests",
			status:  http.StatusTooManyRequests,
			header:  http.Header{"Retry-After": []string{"30"}},
			body:    `{"message":"Too many requests"}`,
			kind:    errs.KindSignal,
			hintHas: "retry",
			hintNot: "munin login github",
		},
		{
			name:    "missing scope",
			status:  http.StatusForbidden,
			header:  http.Header{"X-Ratelimit-Remaining": []string{"4999"}},
			body:    `{"message":"Resource not accessible by personal access token"}`,
			kind:    errs.KindAuth,
			hintHas: "munin login github",
		},
		{
			name:    "bad credentials",
			status:  http.StatusUnauthorized,
			body:    `{"message":"Bad credentials"}`,
			kind:    errs.KindAuth,
			hintHas: "munin login github",
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
	for range 200 {
		got := withJitter(time.Minute)
		if got < time.Minute || got > 75*time.Second {
			t.Fatalf("withJitter(1m) = %s, want within [1m, 1m15s]", got)
		}
	}
	if got := withJitter(0); got != 0 {
		t.Errorf("withJitter(0) = %s, want 0", got)
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
