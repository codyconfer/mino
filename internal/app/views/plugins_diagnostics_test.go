package views

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/mino/internal/plugin"
)

func TestPluginsPageShowsADiagnosticForAnUnregisteredPlugin(t *testing.T) {
	pluginsTestEnv(t)
	plugin.ResetDiagnostics()
	t.Cleanup(plugin.ResetDiagnostics)
	plugin.NoteDiagnostic("test.views.skipped", plugin.KindView, "skipped.view",
		"duplicate view id, so this contribution was skipped")

	kit := testKit(t)
	a := newTestApp(kit.Plugins())
	a = step(a, tea.WindowSizeMsg{Width: 120, Height: 40})
	body := pluginsAnsi.ReplaceAllString(a.View(), "")

	for _, want := range []string{"test.views.skipped", "duplicate view id"} {
		if !strings.Contains(body, want) {
			t.Errorf("plugins page is missing %q; the plugin is absent from the registry, so this row is the "+
				"only place the TUI can tell the user it exists:\n%s", want, body)
		}
	}
}
