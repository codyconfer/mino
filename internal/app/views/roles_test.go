package views

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/deck"
	"github.com/codyconfer/mino/internal/signals"
)

func roleFor(t *testing.T, kit *Kit, name string) *roleView {
	t.Helper()
	var view any
	if name == "" {
		view = kit.RoleBuilder()
	} else {
		view = kit.RoleEditor(name)
	}
	v, ok := view.(*roleView)
	if !ok {
		t.Fatal("role view has the wrong type")
	}
	return v
}

func (v *roleView) set(t *testing.T, key, val string) {
	t.Helper()
	for i := range v.Form().Fields {
		if v.Form().Fields[i].Key == key {
			v.Form().Fields[i].Text = val
			return
		}
	}
	t.Fatalf("role form has no field %q", key)
}

func (v *reportView) set(t *testing.T, key, val string) {
	t.Helper()
	for i := range v.Form().Fields {
		if v.Form().Fields[i].Key == key {
			v.Form().Fields[i].Text = val
			return
		}
	}
	t.Fatalf("report form has no field %q", key)
}

func (v *reportView) selectFlight(t *testing.T, name string) {
	t.Helper()
	for i := range v.Form().Fields {
		if v.Form().Fields[i].Key != "flight" {
			continue
		}
		for opt, o := range v.Form().Fields[i].Options {
			if o == name {
				v.Form().Fields[i].Selected = opt
				return
			}
		}
		t.Fatalf("flight %q is not an option: %v", name, v.Form().Fields[i].Options)
	}
	t.Fatal("report form has no flight field")
}

func stubPreview(kit *Kit, order *[]string) {
	kit.d.PreviewRole = func(rd config.RoleDef, _ time.Duration, body func() app.RolePreviewStep) []app.RolePreviewStep {
		*order = append(*order, "enter")
		step := body()
		*order = append(*order, "exit", "restore")
		return []app.RolePreviewStep{
			{Label: "enter hook (bash)"},
			step,
			{Label: "exit hook (bash)"},
			{Label: "restored role: daily"},
		}
	}
}

func TestRoleEditorPrefillsEveryField(t *testing.T) {
	kit := testKit(t)
	kit.d.App.Directives.Formatters = map[string]config.FormatterDef{
		"brief": {Name: "brief", Template: "a"},
	}
	kit.d.App.Directives.Roles = map[string]config.RoleDef{
		"triage": {
			Name:       "triage",
			Home:       "default",
			Flights:    []string{"default"},
			Queries:    []string{"q1"},
			Formatters: []string{"brief"},
		},
	}

	v := roleFor(t, kit, "triage")
	want := map[string]string{
		"name":       "triage",
		"home":       "default",
		"flights":    "default",
		"queries":    "q1",
		"formatters": "brief",
	}
	for i := range v.Form().Fields {
		f := v.Form().Fields[i]
		w, ok := want[f.Key]
		if !ok {
			continue
		}
		if f.Text != w {
			t.Errorf("field %q = %q, want %q", f.Key, f.Text, w)
		}
		delete(want, f.Key)
	}
	if len(want) != 0 {
		t.Fatalf("role editor is missing fields: %v", want)
	}
}

func TestRoleEditorPreservesContextsHooksAndStatus(t *testing.T) {
	kit := testKit(t)
	base := config.RoleDef{
		Name:     "triage",
		Flights:  []string{"default"},
		Contexts: map[string]string{"kubectl": "prod"},
		Hooks: config.RoleHooks{
			Enter: config.RoleShellHooks{Bash: "echo in"},
			Exit:  config.RoleShellHooks{Bash: "echo out"},
		},
		Status: []config.RoleStatusBlock{{Glyph: "*", Bash: "echo chip"}},
	}
	kit.d.App.Directives.Roles["triage"] = base
	loadKitDirectives(t, kit)

	v := roleFor(t, kit, "triage")
	v.set(t, "queries", "q1")

	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	step(app, tea.KeyMsg{Type: tea.KeyCtrlS})

	directives, err := config.LoadDirectivesFromFiles(kit.d.App.Cfg.Home)
	if err != nil {
		t.Fatal(err)
	}
	rd, ok := directives.Roles["triage"]
	if !ok {
		t.Fatalf("role not written: %+v (status %q)", directives.Roles, v.Status())
	}
	if len(rd.Queries) != 1 || rd.Queries[0] != "q1" {
		t.Errorf("queries = %v, want [q1]", rd.Queries)
	}
	if rd.Contexts["kubectl"] != "prod" {
		t.Errorf("contexts lost on save: %+v", rd.Contexts)
	}
	if rd.Hooks.Enter.Bash != "echo in" || rd.Hooks.Exit.Bash != "echo out" {
		t.Errorf("hooks lost on save: %+v", rd.Hooks)
	}
	if len(rd.Status) != 1 || rd.Status[0].Bash != "echo chip" {
		t.Errorf("status lost on save: %+v", rd.Status)
	}
}

func TestRoleEditorRejectsUnknownReferences(t *testing.T) {
	kit := testKit(t)

	v := roleFor(t, kit, "")
	v.set(t, "flights", "default, nope")
	if _, err := v.role(); err == nil {
		t.Error("a role referencing an unknown flight should fail")
	}

	v = roleFor(t, kit, "")
	v.set(t, "home", "nope")
	if _, err := v.role(); err == nil {
		t.Error("a role with an unknown home flight should fail")
	}

	v = roleFor(t, kit, "")
	v.set(t, "queries", "q1")
	if _, err := v.role(); err != nil {
		t.Errorf("a role with only known references should pass: %v", err)
	}
}

func TestRoleEditorSaveRejectsCollisionAndMissingName(t *testing.T) {
	kit := testKit(t)

	v := roleFor(t, kit, "")
	v.set(t, "flights", "default")
	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	step(app, tea.KeyMsg{Type: tea.KeyCtrlS})
	if !strings.Contains(v.Status(), "name is required") {
		t.Errorf("status = %q, want a name-required message", v.Status())
	}

	v = roleFor(t, kit, "")
	v.set(t, "flights", "default")
	v.set(t, "name", "triage")
	app = deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	step(app, tea.KeyMsg{Type: tea.KeyCtrlS})
	if !strings.Contains(v.Status(), "already exists") {
		t.Errorf("status = %q, want an already-exists message", v.Status())
	}
}

func TestRoleEditorRenameRewritesTheOriginalFile(t *testing.T) {
	kit := testKit(t)
	base := config.RoleDef{Name: "triage", Flights: []string{"default"}}
	if _, _, err := config.SaveDirective(nil, kit.d.App.Cfg.Home, "", config.TypeRole, base.Name, base); err != nil {
		t.Fatal(err)
	}
	loadKitDirectives(t, kit)

	v := roleFor(t, kit, "triage")
	v.set(t, "name", "sifting")

	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	step(app, tea.KeyMsg{Type: tea.KeyCtrlS})

	directives, err := config.LoadDirectivesFromFiles(kit.d.App.Cfg.Home)
	if err != nil {
		t.Fatal(err)
	}
	if _, orphan := directives.Roles["triage"]; orphan {
		t.Errorf("rename left the old role behind: %+v", directives.Roles)
	}
	if _, ok := directives.Roles["sifting"]; !ok {
		t.Fatalf("rename did not write the new role: %+v (status %q)", directives.Roles, v.Status())
	}
}

func TestRoleEditorDryRunReportsStepsAroundTheFlight(t *testing.T) {
	kit := testKit(t)
	var order []string
	stubPreview(kit, &order)

	var gotLabel string
	var gotQueries []string
	kit.d.FetchFlightQueries = func(label string, queries []string) []signals.Section {
		order = append(order, "flight")
		gotLabel, gotQueries = label, queries
		return []signals.Section{{Signal: "github", Title: "t", Items: []signals.Item{{Title: "role hit"}}}}
	}

	kit.d.App.Directives.Roles = map[string]config.RoleDef{
		"triage": {Name: "triage", Home: "default", Flights: []string{"default"}},
	}

	v := roleFor(t, kit, "triage")
	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	app, cmd := update(app, tea.KeyMsg{Type: tea.KeyCtrlR})
	for _, c := range flattenCmds(cmd) {
		if c != nil {
			app = step(app, c())
		}
	}
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 80})

	if gotLabel != "default" || len(gotQueries) != 1 || gotQueries[0] != "q1" {
		t.Fatalf("dry run fetched label=%q queries=%v", gotLabel, gotQueries)
	}
	want := []string{"enter", "flight", "exit", "restore"}
	if len(order) != len(want) {
		t.Fatalf("dry-run order = %v, want %v", order, want)
	}
	for i, w := range want {
		if order[i] != w {
			t.Fatalf("dry-run order = %v, want %v", order, want)
		}
	}

	body := app.View()
	for _, w := range []string{"enter hook", "flight: default", "exit hook", "restored role", "role hit"} {
		if !strings.Contains(body, w) {
			t.Errorf("dry-run results missing %q: %q", w, body)
		}
	}
}

func TestRoleEditorDryRunNeedsAFlight(t *testing.T) {
	kit := testKit(t)
	var order []string
	stubPreview(kit, &order)

	v := roleFor(t, kit, "")
	v.set(t, "queries", "q1")
	if _, _, err := v.editorRun(); err == nil {
		t.Fatal("a role with no flights should not be runnable")
	}
}

func TestRoleEditorValidateAndDelete(t *testing.T) {
	kit := testKit(t)
	rd := config.RoleDef{Name: "doomed", Flights: []string{"default"}}
	if _, _, err := config.SaveDirective(nil, kit.d.App.Cfg.Home, "", config.TypeRole, rd.Name, rd); err != nil {
		t.Fatal(err)
	}
	loadKitDirectives(t, kit)
	path := filepath.Join(kit.d.App.Cfg.Home, "doomed.yaml")

	v := roleFor(t, kit, "doomed")
	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})

	app = step(app, tea.KeyMsg{Type: tea.KeyCtrlT})
	if len(v.Notice()) == 0 {
		t.Error("validate produced no findings panel")
	}

	app = step(app, tea.KeyMsg{Type: tea.KeyCtrlX})
	if !v.Confirming() {
		t.Fatal("delete did not raise a confirmation")
	}
	app = step(app, tea.KeyMsg{Type: tea.KeyLeft})
	app, cmd := update(app, tea.KeyMsg{Type: tea.KeyEnter})
	for _, c := range flattenCmds(cmd) {
		if c != nil {
			app = step(app, c())
		}
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("confirming delete left the role file in place")
	}
}
