package build

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/signals"
)

func TestDetailSignalsListsOnlyDetailCapableSignals(t *testing.T) {
	got := DetailSignals()
	if !slices.Contains(got, "github") {
		t.Errorf("DetailSignals = %v, want it to include github", got)
	}
	for _, name := range got {
		if !plugin.HasCapability(name, plugin.CapDetail) {
			t.Errorf("DetailSignals included %q, which lacks CapDetail", name)
		}
	}
	if !slices.IsSorted(got) {
		t.Errorf("DetailSignals = %v, want sorted", got)
	}
}

func TestDetailRejectsUnknownSignal(t *testing.T) {
	_, err := Detail(context.Background(), "nosuchsignal", signals.Item{}, config.Defaults(), nil, nil)
	if err == nil {
		t.Fatal("want an error")
	}
	if errs.KindOf(err) != errs.KindConfig {
		t.Errorf("kind = %v, want %v", errs.KindOf(err), errs.KindConfig)
	}
}

func TestDetailRejectsSignalWithoutTheCapability(t *testing.T) {
	name := ""
	for _, s := range QueryableSignals() {
		if !plugin.HasCapability(s, plugin.CapDetail) {
			name = s
			break
		}
	}
	if name == "" {
		t.Skip("every queryable signal supports details")
	}
	_, err := Detail(context.Background(), name, signals.Item{}, config.Defaults(), nil, nil)
	if err == nil {
		t.Fatalf("want an error for %q", name)
	}
	if errs.KindOf(err) != errs.KindUsage {
		t.Errorf("kind = %v, want %v", errs.KindOf(err), errs.KindUsage)
	}
	if hint := errs.Hint(err); !strings.Contains(hint, "github") {
		t.Errorf("hint = %q, want it to name the signals that do support details", hint)
	}
}
