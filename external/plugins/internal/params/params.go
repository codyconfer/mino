package params

import (
	"strconv"
	"time"

	"github.com/codyconfer/mino/plugin/stream"
)

const MinPollInterval = stream.MinPollInterval

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
	return stream.PollInterval(params, signal, def)
}
