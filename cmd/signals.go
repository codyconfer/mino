package cmd

import (
	"context"

	"github.com/codyconfer/munin/internal/signals"
	"github.com/codyconfer/munin/internal/signals/build"
)

type errSignal struct {
	name string
	err  error
}

func (e errSignal) Name() string { return e.name }

func (e errSignal) Fetch(context.Context) ([]signals.Section, error) { return nil, e.err }

func buildSignal(name string, params map[string]string) (signals.Signal, error) {
	return build.Signal(name, params, shared.Cfg, shared.Tokens, shared.Cache)
}
