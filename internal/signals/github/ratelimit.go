package github

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/codyconfer/mino/internal/signals"
)

const (
	maxRetryAfter  = signals.MaxRetryAfter
	maxPollBackoff = signals.MaxPollBackoff
)

var (
	retryAfter      = signals.RetryAfter
	withJitter      = signals.WithJitter
	backoffInterval = signals.BackoffInterval
)

func pollIntervalHint(hdr http.Header) time.Duration {
	secs, err := strconv.Atoi(strings.TrimSpace(hdr.Get("X-Poll-Interval")))
	if err != nil || secs <= 0 {
		return 0
	}
	if maxSecs := int(maxRetryAfter / time.Second); secs > maxSecs {
		return maxRetryAfter
	}
	return time.Duration(secs) * time.Second
}

func rateLimited(resp *http.Response, body []byte) bool {
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if resp.StatusCode != http.StatusForbidden {
		return false
	}
	remaining := strings.TrimSpace(resp.Header.Get("X-RateLimit-Remaining"))
	if remaining == "0" {
		return true
	}
	if strings.TrimSpace(resp.Header.Get("Retry-After")) != "" {
		return true
	}
	if remaining != "" {
		return false
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
