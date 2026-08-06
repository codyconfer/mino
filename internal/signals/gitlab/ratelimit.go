package gitlab

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxRetryAfter  = time.Hour
	maxPollBackoff = 15 * time.Minute
	maxBackoffStep = 4
)

func retryAfter(hdr http.Header, now time.Time) (time.Duration, bool) {
	if v := strings.TrimSpace(hdr.Get("Retry-After")); v != "" {
		if secs, err := strconv.Atoi(v); err == nil {
			return boundRetry(time.Duration(secs) * time.Second)
		}
		if t, err := http.ParseTime(v); err == nil {
			return boundRetry(t.Sub(now))
		}
	}
	if strings.TrimSpace(hdr.Get("RateLimit-Remaining")) == "0" {
		if epoch, err := strconv.ParseInt(strings.TrimSpace(hdr.Get("RateLimit-Reset")), 10, 64); err == nil {
			return boundRetry(time.Unix(epoch, 0).Sub(now))
		}
		if t, err := http.ParseTime(strings.TrimSpace(hdr.Get("RateLimit-ResetTime"))); err == nil {
			return boundRetry(t.Sub(now))
		}
	}
	return 0, false
}

func boundRetry(d time.Duration) (time.Duration, bool) {
	if d <= 0 {
		return 0, false
	}
	return min(d, maxRetryAfter), true
}

func rateLimited(resp *http.Response, body []byte) bool {
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if resp.StatusCode != http.StatusForbidden {
		return false
	}
	if strings.TrimSpace(resp.Header.Get("RateLimit-Remaining")) == "0" {
		return true
	}
	if strings.TrimSpace(resp.Header.Get("Retry-After")) != "" {
		return true
	}
	return strings.Contains(strings.ToLower(string(body)), "rate limit")
}

func backoffInterval(base time.Duration, fails int) time.Duration {
	if base <= 0 {
		base = time.Minute
	}
	if fails <= 1 {
		return base
	}
	return min(base*time.Duration(1<<min(fails-1, maxBackoffStep)), maxPollBackoff)
}

func NewRateHint() *RateHint { return &RateHint{} }

type RateHint struct {
	mu   sync.Mutex
	wait time.Duration
	at   time.Time
}

func (h *RateHint) observe(hdr http.Header, now time.Time) {
	if h == nil {
		return
	}
	d, ok := retryAfter(hdr, now)
	if !ok {
		return
	}
	h.mu.Lock()
	h.wait, h.at = d, now
	h.mu.Unlock()
}

func (h *RateHint) delay(now time.Time) time.Duration {
	if h == nil {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.at.IsZero() {
		return 0
	}
	remaining := h.wait - now.Sub(h.at)
	if remaining <= 0 {
		h.at = time.Time{}
		return 0
	}
	return remaining
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
