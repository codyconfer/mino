package plugin_test

import (
	"context"
	"strings"
	"testing"

	"github.com/codyconfer/munin/plugin"
)

func findDiagnostic(t *testing.T, pluginID string, substrings ...string) plugin.Diagnostic {
	t.Helper()
	for _, d := range plugin.Diagnostics() {
		if d.PluginID != pluginID {
			continue
		}
		ok := true
		for _, sub := range substrings {
			if !strings.Contains(d.Message, sub) {
				ok = false
				break
			}
		}
		if ok {
			return d
		}
	}
	var got []string
	for _, d := range plugin.Diagnostics() {
		got = append(got, d.String())
	}
	t.Fatalf("no diagnostic for %q containing %v; have:\n  %s", pluginID, substrings, strings.Join(got, "\n  "))
	return plugin.Diagnostic{}
}

func TestDuplicateSignalRefIsReportedAsDiagnostic(t *testing.T) {
	const good = "diag.good"
	const bad = "diag.bad"
	const signal = "diagsignal"

	plugin.Register(plugin.Descriptor{ID: good, Kind: plugin.KindSignal, Signal: signal})
	plugin.Register(plugin.Descriptor{ID: bad, Kind: plugin.KindSignal, Signal: signal})

	d := findDiagnostic(t, bad, "signal ref", signal, good)
	if d.Kind != plugin.KindSignal || d.Ref != signal {
		t.Errorf("diagnostic = %+v", d)
	}
	if !strings.Contains(d.String(), bad) {
		t.Errorf("String() = %q, want it to name the offending plugin", d.String())
	}
	if len(plugin.DiagnosticsFor(bad)) == 0 {
		t.Error("DiagnosticsFor did not find the collision")
	}
	if !plugin.HasDiagnostics() {
		t.Error("HasDiagnostics = false")
	}
}

func TestDuplicateIDIsReportedAsDiagnostic(t *testing.T) {
	const id = "diag.dupid"
	plugin.Register(plugin.Descriptor{ID: id, Kind: plugin.KindSignal, Signal: "diagdupidone"})
	plugin.Register(plugin.Descriptor{ID: id, Kind: plugin.KindSignal, Signal: "diagdupidtwo"})
	findDiagnostic(t, id, "duplicate plugin id")
}

func TestDiagnosticsAreDeduplicated(t *testing.T) {
	const id = "diag.dedupe"
	plugin.Register(plugin.Descriptor{ID: id, Kind: plugin.KindSignal, Signal: "diagdedupe"})
	before := len(plugin.DiagnosticsFor(id))
	for range 5 {
		plugin.Register(plugin.Descriptor{ID: id, Kind: plugin.KindSignal, Signal: "diagdedupeother"})
	}
	if got := len(plugin.DiagnosticsFor(id)); got != before+1 {
		t.Fatalf("DiagnosticsFor = %d entries, want %d", got, before+1)
	}
}

func TestDuplicateActionIsReportedAsDiagnostic(t *testing.T) {
	const signal = "diagaction"
	plugin.Register(plugin.Descriptor{
		ID:           "diag.action",
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapAction},
	})
	run := func(context.Context, map[string]string) error { return nil }
	plugin.RegisterAction(signal, "go", run)
	plugin.RegisterAction(signal, "go", run)
	findDiagnostic(t, "diag.action", "already registered")
}

func TestDuplicateFilterIsReportedAsDiagnostic(t *testing.T) {
	plugin.Register(plugin.Descriptor{ID: "diag.filter", Kind: plugin.KindSignal, Signal: "diagfilterowner"})
	f := plugin.NamedFilter{Name: "diagfilter", Rules: []plugin.FilterRule{{Field: "title", Include: "x"}}}
	plugin.RegisterFilter("diag.filter", f)
	plugin.RegisterFilter("diag.filter", f)
	findDiagnostic(t, "diag.filter", "already registered")
}

func TestDuplicateBuildersReportedAgainstTheOffender(t *testing.T) {
	const signal = "diagbuilders"
	q := func(plugin.BuildContext) (plugin.Query, error) { return testQuery{name: signal}, nil }
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           "diag.builders.first",
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapQuery},
	}, plugin.Builders{Query: q})
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           "diag.builders.second",
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapQuery},
	}, plugin.Builders{Query: q})

	findDiagnostic(t, "diag.builders.second", "signal ref", signal, "diag.builders.first")
	if len(plugin.DiagnosticsFor("diag.builders.second")) != 1 {
		t.Fatalf("one mistake should produce one diagnostic, got %v",
			plugin.DiagnosticsFor("diag.builders.second"))
	}

	q2, err := plugin.BuildQuery(signal, testBuildCtx{})
	if err != nil || q2 == nil {
		t.Fatalf("incumbent builders lost after a collision: %v, %v", q2, err)
	}
}

func TestEmptyBuildersReportedAsDiagnostic(t *testing.T) {
	plugin.RegisterSignal(plugin.Descriptor{
		ID:     "diag.nobuilders",
		Kind:   plugin.KindSignal,
		Signal: "diagnobuilders",
	}, plugin.Builders{})
	findDiagnostic(t, "diag.nobuilders", "no Query, Stream, or Scheduled builder")
}
