package signals

import (
	"time"

	"github.com/codyconfer/mino/internal/errs"
)

const MinPollInterval = time.Second

func CheckPollInterval(where string, d time.Duration) error {
	if d >= MinPollInterval {
		return nil
	}
	return errs.Newf(errs.KindUsage, "%s: poll interval %s is below the %s minimum", where, d, MinPollInterval).
		WithHint("polling faster than %s burns provider rate limits; GitHub's own X-Poll-Interval floor is 60s", MinPollInterval)
}

func ParsePollInterval(where, raw string) (time.Duration, error) {
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, errs.Newf(errs.KindUsage, "%s: %q is not a valid poll interval", where, raw).
			WithHint("use a Go duration such as 30s, 2m, or 1h")
	}
	return d, CheckPollInterval(where, d)
}
