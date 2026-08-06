package httpx

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
	v := strings.TrimSpace(hdr.Get("Retry-After"))
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(v); err == nil {
		return boundRetry(time.Duration(secs) * time.Second)
	}
	if t, err := http.ParseTime(v); err == nil {
		return boundRetry(t.Sub(now))
	}
	return 0, false
}

func boundRetry(d time.Duration) (time.Duration, bool) {
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

func Backoff(base time.Duration, fails int) time.Duration {
	if base <= 0 {
		base = time.Minute
	}
	if fails <= 1 {
		return base
	}
	return min(base*time.Duration(1<<min(fails-1, maxBackoffStep)), MaxPollBackoff)
}
