package params

import (
	"strconv"
	"time"

	"github.com/codyconfer/munin/external/plugins/internal/errx"
)

const MinPollInterval = time.Second

func Str(params map[string]string, key, def string) string {
	if v := params[key]; v != "" {
		return v
	}
	return def
}

func Int(params map[string]string, key string, def int) int {
	if v := params[key]; v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func Duration(params map[string]string, key string, def time.Duration) time.Duration {
	if v := params[key]; v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func Window(raw string, def time.Duration) time.Duration {
	if raw == "" {
		return def
	}
	if d, err := time.ParseDuration(raw); err == nil {
		return d
	}
	return def
}

func PollInterval(params map[string]string, signal string, def time.Duration) (time.Duration, error) {
	raw := params["interval"]
	if raw == "" {
		return def, nil
	}
	where := signal + ": query param interval"
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, errx.Newf("%s: %q is not a valid poll interval", where, raw).
			WithHint("use a Go duration such as 30s, 2m, or 1h")
	}
	if d < MinPollInterval {
		return 0, errx.Newf("%s: poll interval %s is below the %s minimum", where, d, MinPollInterval).
			WithHint("polling faster than %s burns provider rate limits", MinPollInterval)
	}
	return d, nil
}
