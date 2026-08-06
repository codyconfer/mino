package httpx

import (
	"net/http"
	"testing"
	"time"
)

func TestRetryAfterSecondsForm(t *testing.T) {
	hdr := http.Header{}
	hdr.Set("Retry-After", "45")

	got, ok := RetryAfter(hdr, time.Now())
	if !ok || got != 45*time.Second {
		t.Fatalf("RetryAfter = %s ok=%v, want 45s", got, ok)
	}
}

func TestRetryAfterHTTPDateForm(t *testing.T) {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	hdr := http.Header{}
	hdr.Set("Retry-After", now.Add(90*time.Second).Format(http.TimeFormat))

	got, ok := RetryAfter(hdr, now)
	if !ok || got != 90*time.Second {
		t.Fatalf("RetryAfter = %s ok=%v, want 90s from an HTTP-date header", got, ok)
	}
}

func TestRetryAfterIgnoresThePast(t *testing.T) {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	hdr := http.Header{}
	hdr.Set("Retry-After", now.Add(-time.Minute).Format(http.TimeFormat))

	if got, ok := RetryAfter(hdr, now); ok {
		t.Errorf("RetryAfter = %s ok=true for a date in the past; a negative delay would stall the poller", got)
	}
}

func TestRetryAfterIsCapped(t *testing.T) {
	hdr := http.Header{}
	hdr.Set("Retry-After", "999999")

	got, ok := RetryAfter(hdr, time.Now())
	if !ok || got != MaxRetryAfter {
		t.Fatalf("RetryAfter = %s, want the %s cap; a hostile header must not park the poller for a day",
			got, MaxRetryAfter)
	}
}

func TestRetryAfterAbsentOrJunk(t *testing.T) {
	for _, value := range []string{"", "soon", "  "} {
		hdr := http.Header{}
		if value != "" {
			hdr.Set("Retry-After", value)
		}
		if got, ok := RetryAfter(hdr, time.Now()); ok {
			t.Errorf("RetryAfter(%q) = %s ok=true, want no delay", value, got)
		}
	}
}

func TestBackoffGrowsAndCaps(t *testing.T) {
	base := time.Minute
	if got := Backoff(base, 0); got != base {
		t.Errorf("Backoff(0) = %s, want the base", got)
	}
	if got := Backoff(base, 1); got != base {
		t.Errorf("Backoff(1) = %s, want the base", got)
	}
	if got := Backoff(base, 2); got != 2*base {
		t.Errorf("Backoff(2) = %s, want %s", got, 2*base)
	}
	if got := Backoff(base, 4); got != 8*base {
		t.Errorf("Backoff(4) = %s, want %s", got, 8*base)
	}
	if got := Backoff(base, 50); got != MaxPollBackoff {
		t.Errorf("Backoff(50) = %s, want the %s cap", got, MaxPollBackoff)
	}
}

func TestBackoffSubstitutesAUsableBase(t *testing.T) {
	if got := Backoff(0, 1); got != time.Minute {
		t.Errorf("Backoff with a zero base = %s, want a one-minute substitute rather than a busy loop", got)
	}
}

func TestWithJitterStaysWithinAQuarter(t *testing.T) {
	base := time.Minute
	for range 200 {
		got := WithJitter(base)
		if got < base || got > base+base/4 {
			t.Fatalf("WithJitter = %s, want it within [%s, %s]; jitter exists to desynchronize pollers, "+
				"not to change the interval", got, base, base+base/4)
		}
	}
	if got := WithJitter(0); got != 0 {
		t.Errorf("WithJitter(0) = %s, want 0", got)
	}
	if got := WithJitter(-time.Second); got != -time.Second {
		t.Errorf("WithJitter(-1s) = %s, want the input unchanged", got)
	}
}
