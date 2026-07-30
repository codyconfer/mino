package views

import (
	"testing"

	"github.com/codyconfer/munin/internal/plugin"
)

func TestPluginsPageWillNotToggleADiagnosticRow(t *testing.T) {
	pluginsTestEnv(t)
	plugin.ResetDiagnostics()
	t.Cleanup(plugin.ResetDiagnostics)
	plugin.NoteDiagnostic("test.views.orphan", plugin.KindSignal, "orphan.signal", "registration was skipped")

	page := &pluginsPage{kit: testKit(t)}
	page.reload()

	orphan := -1
	for i, row := range page.rows {
		if row.orphan {
			orphan = i
		}
	}
	if orphan < 0 {
		t.Fatal("reload produced no diagnostic row, so this test cannot check that one is unselectable")
	}

	page.cursor = orphan
	if _, ok := page.selected(); ok {
		t.Error("a diagnostic row reported itself as selected; enter would call SetEnabled on a plugin that " +
			"is not in the registry, which can only fail")
	}
}

func TestPluginsPageCursorSkipsDiagnosticRows(t *testing.T) {
	pluginsTestEnv(t)
	plugin.ResetDiagnostics()
	t.Cleanup(plugin.ResetDiagnostics)
	plugin.NoteDiagnostic("test.views.skipme", plugin.KindSignal, "skipme.signal", "registration was skipped")

	page := &pluginsPage{kit: testKit(t)}
	page.reload()
	if len(page.rows) < 2 {
		t.Skipf("need a registered row plus a diagnostic row; got %d", len(page.rows))
	}
	if !page.rows[len(page.rows)-1].orphan {
		t.Fatal("expected the diagnostic row to sort last")
	}

	page.cursor = 0
	for range len(page.rows) + 2 {
		page.cursor = page.nextSelectable(page.cursor, 1)
		if page.rows[page.cursor].orphan {
			t.Fatalf("cursor landed on diagnostic row %d; moving down must skip rows that cannot be acted on",
				page.cursor)
		}
	}
}
