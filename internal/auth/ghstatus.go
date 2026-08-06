package auth

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/codyconfer/mino/internal/errs"
)

const maxRetryAfterHeader = time.Hour

const githubScopeHint = "your GitHub credential may lack the required scopes; run `mino login github`, set $GITHUB_TOKEN, or check the GitHub App's installation permissions"

func classifyGitHubStatus(resp *http.Response, msg string) error {
	statusErr := func(kind errs.Kind) *errs.Error {
		return errs.Newf(kind, "github api %s: %s", resp.Status, msg)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return statusErr(errs.KindAuth).WithHint("%s", githubScopeHint)
	case githubRateLimited(resp, msg):
		e := statusErr(errs.KindSignal)
		if d, ok := retryAfterHeader(resp.Header, time.Now()); ok {
			return e.WithHint("github rate limit reached; retry after %s", d.Round(time.Second))
		}
		return e.WithHint("github rate limit reached; retry in a few minutes")
	case resp.StatusCode == http.StatusForbidden:
		if hint := githubRestrictionHint(msg); hint != "" {
			return statusErr(errs.KindAuth).WithHint("%s", hint)
		}
		return statusErr(errs.KindAuth).WithHint("%s", githubScopeHint)
	default:
		return statusErr(errs.KindSignal)
	}
}

func githubRateLimited(resp *http.Response, msg string) bool {
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
	return strings.Contains(strings.ToLower(msg), "rate limit")
}

func githubRestrictionHint(msg string) string {
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "saml"), strings.Contains(low, "single sign-on"):
		return "the organization enforces SAML single sign-on; authorize this token for the org, then retry"
	case strings.Contains(low, "ip allow"), strings.Contains(low, "allow list"), strings.Contains(low, "allowlist"):
		return "the organization restricts API access by IP allow list; connect from an approved network or ask an owner to add your address"
	}
	return ""
}

func retryAfterHeader(hdr http.Header, now time.Time) (time.Duration, bool) {
	if v := strings.TrimSpace(hdr.Get("Retry-After")); v != "" {
		if secs, err := strconv.Atoi(v); err == nil {
			return boundRetryAfterHeader(time.Duration(secs) * time.Second)
		}
		if t, err := http.ParseTime(v); err == nil {
			return boundRetryAfterHeader(t.Sub(now))
		}
	}
	if strings.TrimSpace(hdr.Get("X-RateLimit-Remaining")) == "0" {
		if epoch, err := strconv.ParseInt(strings.TrimSpace(hdr.Get("X-RateLimit-Reset")), 10, 64); err == nil {
			return boundRetryAfterHeader(time.Unix(epoch, 0).Sub(now))
		}
	}
	return 0, false
}

func boundRetryAfterHeader(d time.Duration) (time.Duration, bool) {
	if d <= 0 {
		return 0, false
	}
	return min(d, maxRetryAfterHeader), true
}
