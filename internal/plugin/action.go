package plugin

import (
	"context"

	pub "github.com/codyconfer/mino/plugin"
)

type ActionFunc = pub.ActionFunc
type ActionSpec = pub.ActionSpec

func init() {
	pub.SetSignalEnabledFunc(SignalEnabled)
	pub.SetPluginEnabledFunc(Enabled)
}

func RegisterAction(signal, name string, run ActionFunc, opts ...Option) {
	pub.RegisterAction(signal, name, run, opts...)
}

func LookupAction(signal, name string) (ActionSpec, bool) {
	return pub.LookupAction(signal, name)
}

func ActionsFor(signal string) []ActionSpec { return pub.ActionsFor(signal) }

func RunAction(ctx context.Context, signal, name string, params map[string]string) error {
	return pub.RunAction(ctx, signal, name, params)
}
