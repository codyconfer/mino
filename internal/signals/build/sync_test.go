package build

import (
	"testing"

	"github.com/codyconfer/munin/internal/plugin"
)

func TestPluginAndBuilderRegistriesAligned(t *testing.T) {
	plugin.RegisterBuiltins()
	builders := BuilderSignals()
	for sig := range plugin.KnownSignals() {
		if !plugin.SignalEnabled(sig) {
			continue
		}
		if !builders[sig] {
			t.Errorf("plugin registry has enabled signal %q with no build.registry entry", sig)
		}
	}
	for sig := range builders {
		if !plugin.KnownSignals()[sig] {
			t.Errorf("build.registry has signal %q missing from plugin registry", sig)
		}
	}
}

func TestCapActionHasRegisteredActions(t *testing.T) {
	plugin.RegisterBuiltins()
	for _, d := range plugin.All() {
		if d.Kind != plugin.KindSignal || d.Signal == "" {
			continue
		}
		if !plugin.HasCapability(d.Signal, plugin.CapAction) {
			continue
		}
		if len(plugin.ActionsFor(d.Signal)) == 0 {
			t.Errorf("signal %q advertises CapAction but has no RegisterAction bindings", d.Signal)
		}
	}
}

func TestCapStreamHasActiveBuilder(t *testing.T) {
	plugin.RegisterBuiltins()
	for _, d := range plugin.All() {
		if d.Kind != plugin.KindSignal || d.Signal == "" {
			continue
		}
		if !plugin.HasCapability(d.Signal, plugin.CapStream) {
			continue
		}
		if !HasActiveBuilder(d.Signal) {
			t.Errorf("signal %q advertises CapStream but has no active builder", d.Signal)
		}
	}
}
