package cmd

import (
	"context"
	"strconv"
	"time"

	"github.com/codyconfer/munin/internal/auth"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/signals"
	"github.com/codyconfer/munin/internal/signals/demo"
)

func googleAuth() auth.GoogleAuth {
	return auth.GoogleAuth{
		Store:        shared.tokens,
		ClientID:     shared.cfg.Google.OAuthClientID,
		ClientSecret: shared.cfg.Google.OAuthClientSecret,
	}
}

type errSignal struct {
	name string
	err  error
}

func (e errSignal) Name() string { return e.name }

func (e errSignal) Fetch(context.Context) ([]signals.Section, error) { return nil, e.err }

func paramStr(params map[string]string, key, def string) string {
	if v := params[key]; v != "" {
		return v
	}
	return def
}

func paramInt(params map[string]string, key string, def int) int {
	if v := params[key]; v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func paramDuration(params map[string]string, key string, def time.Duration) time.Duration {
	if v := params[key]; v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func buildSignal(name string, params map[string]string) (signals.Signal, error) {
	switch name {
	case "demo":
		return demo.Signal{}, nil
	case "github":
		return buildGithub(params)
	case "calendar":
		return buildCalendar(params)
	case "gmail":
		return buildGmail(params)
	case "docs":
		return buildDocs(params)
	case "drive":
		return buildDrive(params)
	case "tasks":
		return buildTasks(params)
	case "slack":
		return buildSlack(params)
	default:
		return nil, errs.Newf(errs.KindConfig, "unknown signal %q", name)
	}
}
