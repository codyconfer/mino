package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/plugin"
)

func runPluginsList(t *testing.T) string {
	t.Helper()
	var out bytes.Buffer
	root := &cobra.Command{Use: "munin", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(newPluginsCmd())
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"plugins", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("plugins list: %v", err)
	}
	return out.String()
}

func TestPluginsListSurfacesDiagnostics(t *testing.T) {
	useServeTestApp(t, "")
	plugin.ResetDiagnostics()
	t.Cleanup(plugin.ResetDiagnostics)
	plugin.NoteDiagnostic("test.cmd.skipped", plugin.KindView, "skipped.view",
		"duplicate view id, so this contribution was skipped")

	out := runPluginsList(t)
	if !strings.Contains(out, "problem:") {
		t.Fatalf("plugins list printed no problem line:\n%s", out)
	}
	for _, want := range []string{"test.cmd.skipped", "skipped.view", "duplicate view id"} {
		if !strings.Contains(out, want) {
			t.Errorf("plugins list output is missing %q; a skipped contribution is not in the registry, so "+
				"enable/disable/install all answer \"unknown plugin\" and the listing is the only place it "+
				"can be named:\n%s", want, out)
		}
	}
}

func TestPluginsListStaysQuietWithNoDiagnostics(t *testing.T) {
	useServeTestApp(t, "")
	plugin.ResetDiagnostics()
	t.Cleanup(plugin.ResetDiagnostics)

	if out := runPluginsList(t); strings.Contains(out, "problem:") {
		t.Errorf("plugins list printed a problem line with no diagnostics recorded:\n%s", out)
	}
}
