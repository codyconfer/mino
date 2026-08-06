package signals

import (
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	MaxRetryAfter  = time.Hour
	MaxPollBackoff = 15 * time.Minute

	maxBackoffStep = 8
)

func RetryAfter(hdr http.Header, now time.Time) (time.Duration, bool) {
	if v := strings.TrimSpace(hdr.Get("Retry-After")); v != "" {
		if secs, err := strconv.Atoi(v); err == nil {
			return BoundRetryAfter(time.Duration(secs) * time.Second)
		}
		if t, err := http.ParseTime(v); err == nil {
			return BoundRetryAfter(t.Sub(now))
		}
	}
	if strings.TrimSpace(hdr.Get("X-RateLimit-Remaining")) == "0" {
		if epoch, err := strconv.ParseInt(strings.TrimSpace(hdr.Get("X-RateLimit-Reset")), 10, 64); err == nil {
			return BoundRetryAfter(time.Unix(epoch, 0).Sub(now))
		}
	}
	return 0, false
}

func BoundRetryAfter(d time.Duration) (time.Duration, bool) {
	if d <= 0 {
		return 0, false
	}
	return min(d, MaxRetryAfter), true
}

func WithJitter(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	return d + time.Duration(rand.Int64N(int64(d)/4+1))
}

func BackoffInterval(base time.Duration, fails int) time.Duration {
	if base <= 0 {
		base = time.Minute
	}
	if fails <= 1 {
		return base
	}
	return min(base*time.Duration(1<<min(fails-1, maxBackoffStep)), MaxPollBackoff)
}
