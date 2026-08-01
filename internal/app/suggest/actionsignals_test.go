package suggest

import (
	"slices"
	"testing"

	"github.com/codyconfer/mino/internal/plugin"
)

func TestActionSignalsOnlyOffersSignalsThatAdvertiseCapAction(t *testing.T) {
	plugin.RegisterBuiltins()
	all := Signals()
	acts := ActionSignals()

	if len(all) == 0 {
		t.Fatal("no signals registered; the fixture cannot distinguish anything")
	}
	var withoutAction []string
	for _, sig := range all {
		if !plugin.HasCapability(sig, plugin.CapAction) {
			withoutAction = append(withoutAction, sig)
		}
	}
	if len(withoutAction) == 0 {
		t.Fatalf("every registered signal advertises CapAction (%v), so this test cannot tell a filtered "+
			"completion from an unfiltered one; give it a signal without CapAction", all)
	}

	for _, sig := range withoutAction {
		if slices.Contains(acts, sig) {
			t.Errorf("ActionSignals offered %q, which does not advertise CapAction; `action list` filters on "+
				"CapAction, so completing it hands the user an argument that prints nothing", sig)
		}
	}
	for _, sig := range all {
		if plugin.HasCapability(sig, plugin.CapAction) && !slices.Contains(acts, sig) {
			t.Errorf("ActionSignals omitted %q even though it advertises CapAction", sig)
		}
	}
	if !slices.IsSorted(acts) {
		t.Errorf("ActionSignals = %v; want sorted output so completion order is stable", acts)
	}
}
