package stream

import (
	"time"

	"github.com/codyconfer/mino/plugin"
)

const MinPollInterval = time.Second

func CheckPollInterval(where string, d time.Duration) error {
	if d >= MinPollInterval {
		return nil
	}
	return plugin.NewErrorf("%s: poll interval %s is below the %s minimum", where, d, MinPollInterval).
		WithHint("polling faster than %s burns provider rate limits", MinPollInterval)
}

func ParsePollInterval(where, raw string) (time.Duration, error) {
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, plugin.NewErrorf("%s: %q is not a valid poll interval", where, raw).
			WithHint("use a Go duration such as 30s, 2m, or 1h")
	}
	return d, CheckPollInterval(where, d)
}

func PollInterval(params map[string]string, signal string, def time.Duration) (time.Duration, error) {
	raw := params["interval"]
	if raw == "" {
		return def, nil
	}
	return ParsePollInterval(signal+": query param interval", raw)
}
