package signals

import (
	"errors"
	"time"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/plugin"
	"github.com/codyconfer/mino/plugin/stream"
)

const MinPollInterval = stream.MinPollInterval

func CheckPollInterval(where string, d time.Duration) error {
	err := usageError(stream.CheckPollInterval(where, d))
	var e *errs.Error
	if errors.As(err, &e) && e.Hint != "" {
		return e.WithHint("%s; GitHub's own X-Poll-Interval floor is 60s", e.Hint)
	}
	return err
}

func ParsePollInterval(where, raw string) (time.Duration, error) {
	d, err := stream.ParsePollInterval(where, raw)
	if err == nil {
		return d, nil
	}
	if _, perr := time.ParseDuration(raw); perr != nil {
		return d, usageError(err)
	}
	return d, CheckPollInterval(where, d)
}

func usageError(err error) error {
	if err == nil {
		return nil
	}
	var e *plugin.Error
	if !errors.As(err, &e) {
		return err
	}
	out := errs.New(errs.KindUsage, e.Message())
	if hint := e.Hint(); hint != "" {
		out = out.WithHint("%s", hint)
	}
	return out
}
