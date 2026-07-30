package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/codyconfer/munin/plugin"
)

// TestRegistrationIsNotTruncatedByABadContribution is the gap left by
// TestRunSurvivesPanickingPluginRegistration: the CLI surviving is not enough,
// the contributions registered after the mistake must still land.
func TestRegistrationIsNotTruncatedByABadContribution(t *testing.T) {
	ran := false
	opts := Options{
		Args: []string{"--help"},
		RegisterPlugins: func() {
			plugin.RegisterSignal(plugin.Descriptor{
				ID:           "apptest.trunc.a",
				Kind:         plugin.KindSignal,
				Signal:       "apptesttrunca",
				Capabilities: []plugin.Capability{plugin.CapQuery, plugin.CapAction},
			}, plugin.Builders{
				Query: func(plugin.BuildContext) (plugin.Query, error) {
					return nil, errors.New("apptest: not built in this test")
				},
			})
			// A bad SDK call from one plugin: it must be skipped, not fatal.
			plugin.RegisterView("", "", nil)
			plugin.RegisterAction("apptesttrunca", "go", func(context.Context, map[string]string) error {
				return nil
			})
			plugin.Register(plugin.Descriptor{
				ID:     "apptest.trunc.b",
				Kind:   plugin.KindSignal,
				Signal: "apptesttruncb",
			})
		},
		CLI: func(context.Context, []string) error {
			ran = true
			return nil
		},
	}
	if err := Run(opts); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	if !ran {
		t.Fatal("CLI never ran")
	}
	if _, ok := plugin.LookupAction("apptesttrunca", "go"); !ok {
		t.Error("the contribution registered after the bad one was dropped")
	}
	if _, ok := plugin.Lookup("apptest.trunc.b"); !ok {
		t.Error("plugins registered after the bad contribution vanished: registration was truncated")
	}
}

func TestPanickingRegistrationNamesTheOffenderAndSaysItTruncated(t *testing.T) {
	const owner = "apptest.panic.owner"
	opts := Options{
		Args: []string{"--help"},
		RegisterPlugins: func() {
			plugin.Register(plugin.Descriptor{
				ID:     owner,
				Kind:   plugin.KindSignal,
				Signal: "apptestpanicowner",
			})
			panic("boom inside the plugin's own Register body")
		},
		CLI: func(context.Context, []string) error { return nil },
	}
	if err := Run(opts); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	for _, d := range plugin.Diagnostics() {
		if !strings.Contains(d.Message, "panicked") {
			continue
		}
		if !strings.Contains(d.Message, "boom inside the plugin's own Register body") {
			continue
		}
		if !strings.Contains(d.String(), owner) {
			continue
		}
		if !strings.Contains(d.Message, "truncated") {
			continue
		}
		return
	}
	var got []string
	for _, d := range plugin.Diagnostics() {
		got = append(got, d.String())
	}
	t.Fatalf("the recovered panic must name the plugin that was registering and say registration was truncated; have:\n  %s",
		strings.Join(got, "\n  "))
}

func TestDiagnosticReportSuppression(t *testing.T) {
	if !reportDiagnosticsFor([]string{"plugins", "list"}) {
		t.Error("plugin problems must be reported for an ordinary command")
	}
	for _, args := range [][]string{
		{"__complete", "query", ""},
		{"__completeNoDesc", "fly"},
		{"completion", "zsh"},
	} {
		if reportDiagnosticsFor(args) {
			t.Errorf("diagnostics leak into shell completion output: %v", args)
		}
	}
	t.Setenv(EnvPluginDiagnostics, "off")
	if reportDiagnosticsFor([]string{"plugins", "list"}) {
		t.Errorf("%s=off did not suppress the report", EnvPluginDiagnostics)
	}
}
