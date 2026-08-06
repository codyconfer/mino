package gitea

import (
	"net/http"
	"time"

	"github.com/codyconfer/mino/internal/signals"
)

var (
	retryAfter      = signals.RetryAfter
	withJitter      = signals.WithJitter
	backoffInterval = signals.BackoffInterval
)

func rateLimited(resp *http.Response) bool {
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	return resp.StatusCode == http.StatusForbidden && resp.Header.Get("Retry-After") != ""
}

func nextPollInterval(base time.Duration, resp *http.Response) time.Duration {
	next := base
	if d, ok := retryAfter(resp.Header, time.Now()); ok {
		if d = withJitter(d); d > next {
			next = d
		}
	}
	return next
}
