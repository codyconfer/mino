package cmd

import (
	"github.com/codyconfer/mino/internal/app/run"
	"github.com/codyconfer/mino/internal/signals"
)

func buildSignal(name string, params map[string]string) (signals.Signal, error) {
	return run.Signal(shared, name, params)
}
