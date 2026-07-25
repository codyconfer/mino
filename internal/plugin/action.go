package plugin

import (
	"context"

	pub "github.com/codyconfer/munin/plugin"
)

type ActionFunc = pub.ActionFunc
type ActionSpec = pub.ActionSpec

func init() {
	pub.SetSignalEnabledFunc(SignalEnabled)
	pub.SetPluginEnabledFunc(Enabled)
}

// RegisterAction registers a CapAction implementation for signal/name.
func RegisterAction(signal, name string, run ActionFunc) {
	pub.RegisterAction(signal, name, run)
}

// LookupAction returns a registered action.
func LookupAction(signal, name string) (ActionSpec, bool) {
	return pub.LookupAction(signal, name)
}

// ActionsFor lists actions registered for signal, sorted by name.
func ActionsFor(signal string) []ActionSpec { return pub.ActionsFor(signal) }

// RunAction executes a registered action.
func RunAction(ctx context.Context, signal, name string, params map[string]string) error {
	return pub.RunAction(ctx, signal, name, params)
}
