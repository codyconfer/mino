package github

import (
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	maxRetryAfter  = time.Hour
	maxPollBackoff = 15 * time.Minute
	maxBackoffStep = 8
)

func pollIntervalHint(hdr http.Header) time.Duration {
	secs, err := strconv.Atoi(strings.TrimSpace(hdr.Get("X-Poll-Interval")))
	if err != nil || secs <= 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}

func retryAfter(hdr http.Header, now time.Time) (time.Duration, bool) {
	if v := strings.TrimSpace(hdr.Get("Retry-After")); v != "" {
		if secs, err := strconv.Atoi(v); err == nil {
			return boundRetry(time.Duration(secs) * time.Second)
		}
		if t, err := http.ParseTime(v); err == nil {
			return boundRetry(t.Sub(now))
		}
	}
	if strings.TrimSpace(hdr.Get("X-RateLimit-Remaining")) == "0" {
		if epoch, err := strconv.ParseInt(strings.TrimSpace(hdr.Get("X-RateLimit-Reset")), 10, 64); err == nil {
			return boundRetry(time.Unix(epoch, 0).Sub(now))
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

func withJitter(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	return d + time.Duration(rand.Int64N(int64(d)/4+1))
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

func rateLimited(resp *http.Response, body []byte) bool {
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if resp.StatusCode != http.StatusForbidden {
		return false
	}
	if strings.TrimSpace(resp.Header.Get("X-RateLimit-Remaining")) == "0" {
		return true
	}
	if strings.TrimSpace(resp.Header.Get("Retry-After")) != "" {
		return true
	}
	return strings.Contains(strings.ToLower(string(body)), "rate limit")
}

func restrictionHint(body []byte) string {
	low := strings.ToLower(string(body))
	switch {
	case strings.Contains(low, "saml"), strings.Contains(low, "single sign-on"):
		return "the organization enforces SAML single sign-on; authorize this token for the org, then retry"
	case strings.Contains(low, "ip allow"), strings.Contains(low, "allow list"), strings.Contains(low, "allowlist"):
		return "the organization restricts API access by IP allow list; connect from an approved network or ask an owner to add your address"
	}
	return ""
}
