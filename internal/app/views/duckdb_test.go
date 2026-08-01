package views

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/mino/internal/config"
)

func duckDBFor(t *testing.T, kit *Kit, name string) *duckDBView {
	t.Helper()
	var view any
	if name == "" {
		view = kit.DuckDBBuilder()
	} else {
		view = kit.DuckDBEditor(name)
	}
	v, ok := view.(*duckDBView)
	if !ok {
		t.Fatal("DuckDB editor has the wrong type")
	}
	return v
}

func (v *duckDBView) set(t *testing.T, key, value string) {
	t.Helper()
	for i := range v.Form().Fields {
		if v.Form().Fields[i].Key == key {
			v.Form().Fields[i].Text = value
			return
		}
	}
	t.Fatalf("DuckDB form has no field %q", key)
}

func TestMainMenuRoutesThroughDuckDBQueryLibrary(t *testing.T) {
	kit := testKit(t)
	var itemFound bool
	for _, item := range kit.mainMenuItems() {
		if item.Label == "Query DuckDB" {
			t.Fatal("main menu still exposes the legacy DuckDB label")
		}
		if item.Label == "DuckDB" {
			itemFound = true
			if !strings.Contains(item.Desc, "save") {
				t.Fatalf("DuckDB description does not mention saved queries: %q", item.Desc)
			}
		}
	}
	if !itemFound {
		t.Fatal("main menu has no DuckDB entry")
	}

	kit.d.App.Directives.DuckDB = map[string]config.DuckDBQuery{
		"recent-runs": {Name: "recent-runs", Database: "audit", SQL: "SELECT * FROM runs"},
	}
	app := newTestApp(kit.DuckDB())
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	body := app.View()
	for _, want := range []string{"duckdb", "New", "recent-runs", "audit", "SELECT * FROM runs"} {
		if !strings.Contains(body, want) {
			t.Fatalf("DuckDB menu missing %q: %q", want, body)
		}
	}
}

func TestDuckDBBuilderUsesSharedEditorAndRunsAsynchronously(t *testing.T) {
	kit := testKit(t)
	v := duckDBFor(t, kit, "")
	v.set(t, "sql", "SELECT 42 AS answer")
	var gotPath, gotSQL string
	v.exec = func(path, sql string) auditResult {
		gotPath, gotSQL = path, sql
		return auditResult{ran: true, cols: []string{"answer"}, rows: [][]string{{"42"}}}
	}

	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	before := app.View()
	for _, want := range []string{"build DuckDB query", "database", "read-only SQL", "name (required to save)"} {
		if !strings.Contains(before, want) {
			t.Fatalf("DuckDB editor missing %q: %q", want, before)
		}
	}

	app, cmd := update(app, tea.KeyMsg{Type: tea.KeyCtrlR})
	if gotSQL != "" {
		t.Fatal("DuckDB query ran inside the update loop")
	}
	if !v.Running() || cmd == nil {
		t.Fatal("DuckDB editor did not enter the asynchronous running state")
	}
	for _, c := range flattenCmds(cmd) {
		if c != nil {
			app = step(app, c())
		}
	}
	if gotSQL != "SELECT 42 AS answer" || filepath.Base(gotPath) != "audit.duckdb" {
		t.Fatalf("execution = %q against %q", gotSQL, gotPath)
	}
	body := app.View()
	for _, want := range []string{"results: ad-hoc", "answer=42", "read-only SQL"} {
		if !strings.Contains(body, want) {
			t.Fatalf("framed DuckDB results missing %q: %q", want, body)
		}
	}
}

func TestDuckDBBuilderSavesAndEditorPrefills(t *testing.T) {
	kit := testKit(t)
	v := duckDBFor(t, kit, "")
	v.set(t, "sql", "SELECT name, count FROM runs")
	v.set(t, "name", "run-counts")

	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	step(app, tea.KeyMsg{Type: tea.KeyCtrlS})

	path := filepath.Join(kit.d.App.Cfg.Home, config.DirDuckDB, "run-counts.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("saved DuckDB query missing: %v (status %q)", err, v.Status())
	}
	for _, want := range []string{"name: run-counts", "type: duckdb", "database: audit", "SELECT name, count FROM runs"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("saved DuckDB query missing %q:\n%s", want, raw)
		}
	}

	dirs, err := config.LoadDirectivesFromFiles(kit.d.App.Cfg.Home)
	if err != nil {
		t.Fatal(err)
	}
	kit.d.App.Directives = dirs
	edited := duckDBFor(t, kit, "run-counts")
	if edited.Value("sql") != "SELECT name, count FROM runs" || edited.database() != "audit" {
		t.Fatalf("saved query did not prefill: sql=%q database=%q", edited.Value("sql"), edited.database())
	}
}

func TestDuckDBBuilderRejectsWrites(t *testing.T) {
	v := duckDBFor(t, testKit(t), "")
	v.set(t, "sql", "DELETE FROM runs")
	if _, err := v.editorValue(); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("write validation error = %v", err)
	}
}
