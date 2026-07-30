package verify

import (
	"strings"
	"testing"

	"github.com/codyconfer/munin/internal/plugin"
)

func TestPluginsReportsRegistryDiagnostics(t *testing.T) {
	plugin.ResetDiagnostics()
	t.Cleanup(plugin.ResetDiagnostics)
	plugin.NoteDiagnostic("test.verify.skipped", plugin.KindSignal, "skippedsignal",
		"registration was skipped by a collision guard")

	var got *Finding
	for _, f := range Plugins() {
		if f.Name == "test.verify.skipped" {
			f := f
			got = &f
		}
	}
	if got == nil {
		t.Fatal("verify plugins reported no finding for a skipped registration; a skipped contribution is " +
			"absent from the registry, so verify and the listings are the only places it can surface at all")
	}
	if got.OK || got.Warn {
		t.Errorf("finding = {OK:%v Warn:%v}; a skipped contribution must count as a problem so `munin verify` "+
			"exits non-zero", got.OK, got.Warn)
	}
	if !strings.Contains(got.Msg, "collision guard") {
		t.Errorf("finding msg = %q; want the diagnostic message carried through", got.Msg)
	}
	if !strings.Contains(got.Msg, "skippedsignal") {
		t.Errorf("finding msg = %q; want the ref named so the user knows which contribution was dropped", got.Msg)
	}
}

func TestPluginsReportsAnUnattributedDiagnostic(t *testing.T) {
	plugin.ResetDiagnostics()
	t.Cleanup(plugin.ResetDiagnostics)
	plugin.NoteDiagnostic("", "", "", "registration panicked before any id was known")

	for _, f := range Plugins() {
		if f.Name == "<unidentified plugin>" && !f.OK {
			return
		}
	}
	t.Error("a diagnostic with no plugin id vanished from verify; it is the one case where the user has no " +
		"other way to learn a plugin failed to register")
}
