package cmd

import (
	"testing"

	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/signals/build"
)

func TestActionListCompletesOnlyActionCapableSignals(t *testing.T) {
	useServeTestApp(t, "")
	_ = build.KnownSignals()
	plugin.RegisterBuiltins()

	c := newActionListCmd()
	if c.ValidArgsFunction == nil {
		t.Fatal("action list has no completion function")
	}
	got, _ := c.ValidArgsFunction(c, nil, "")
	if len(got) == 0 {
		t.Fatal("action list completed nothing; at least one built-in signal advertises CapAction")
	}
	for _, sig := range got {
		if !plugin.HasCapability(sig, plugin.CapAction) {
			t.Errorf("action list completed %q, which has no CapAction; `action list %s` filters on CapAction "+
				"and would print \"no actions for signal\", so the completion is offering a dead end", sig, sig)
		}
	}
}

func TestActionRunCompletesOnlyActionCapableSignals(t *testing.T) {
	useServeTestApp(t, "")
	_ = build.KnownSignals()
	plugin.RegisterBuiltins()

	got, _ := completeActionRun(nil, nil, "")
	if len(got) == 0 {
		t.Fatal("action run completed no signals")
	}
	for _, sig := range got {
		if !plugin.HasCapability(sig, plugin.CapAction) {
			t.Errorf("action run completed %q, which has no CapAction; build.Action would reject it", sig)
		}
	}
}
