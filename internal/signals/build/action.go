package build

import (
	"context"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/plugin"
)

// Action runs a registered CapAction for signal/name.
func Action(ctx context.Context, signal, name string, params map[string]string) error {
	if !HasBuilder(signal) {
		return errs.Newf(errs.KindConfig, "unknown signal %q", signal)
	}
	if !plugin.HasCapability(signal, plugin.CapAction) {
		return errs.Newf(errs.KindUsage, "signal %q has no CapAction", signal)
	}
	if err := plugin.RunAction(ctx, signal, name, params); err != nil {
		return errs.Wrapf(errs.KindSignal, err, "action %s/%s", signal, name)
	}
	return nil
}

// Actions lists registered action names for signal.
func Actions(signal string) []plugin.ActionSpec {
	return plugin.ActionsFor(signal)
}
