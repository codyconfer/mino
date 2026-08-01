package views

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/forms"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/audit"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/deck"
	"github.com/codyconfer/mino/internal/filter"
	"github.com/codyconfer/mino/internal/keymap"
	"github.com/codyconfer/mino/internal/plugin"
	"github.com/codyconfer/mino/internal/signals"
	"github.com/codyconfer/mino/internal/testenv"
	pub "github.com/codyconfer/mino/plugin"
)

func TestMain(m *testing.M) {
	keymap.Register()
	os.Exit(m.Run())
}

// newTestApp builds a deck model carrying mino's scope, as cmd does in
// production via app.BuildScope.
func newTestApp(root vkdeck.View, opts ...deck.Option) *vkdeck.Model {
	scope := app.BuildScope("", keymap.DefaultSchemeKey)
	return deck.New(root, append([]deck.Option{deck.WithScope(scope)}, opts...)...)
}

func testKit(t *testing.T) *Kit {
	t.Helper()
	testenv.Isolate(t)
	cfg := config.Defaults()
	cfg.Home = t.TempDir()
	directives := &config.Directives{
		Queries: map[string]config.Query{
			"q1": {Name: "q1", Signal: "github"},
			"f1": {Name: "f1", Rules: []filter.Rule{{Exclude: "bot$"}}},
		},
		Flights: map[string]config.Flight{"default": {Name: "default", Queries: []string{"q1"}}},
		Roles:   map[string]config.RoleDef{"triage": {Name: "triage", Flights: []string{"default"}}},
	}
	return New(Deps{
		App:                &app.App{Cfg: cfg, Directives: directives},
		Scope:              app.BuildScope("", keymap.DefaultSchemeKey),
		FetchQuery:         func(string) []signals.Section { return nil },
		FetchFlightAudited: func(string) []signals.Section { return nil },
		FetchFlightQueries: func(string, []string) []signals.Section { return nil },
		Verify:             func(string) []Finding { return nil },
		ExportDirectives:   func() ([]string, error) { return nil, nil },
	})
}

func loadKitDirectives(t *testing.T, kit *Kit) {
	t.Helper()
	home := kit.d.App.Cfg.Home
	cur := kit.d.App.Directives
	for name, q := range cur.Queries {
		if cur.Source(q.Kind(), name) != "" {
			continue
		}
		if _, _, err := config.SaveDirective(nil, home, "", q.Kind(), name, q); err != nil {
			t.Fatal(err)
		}
	}
	for name, fl := range cur.Flights {
		if _, _, err := config.SaveDirective(nil, home, "", config.TypeFlight, name, fl); err != nil {
			t.Fatal(err)
		}
	}
	for name, rd := range cur.Roles {
		if _, _, err := config.SaveDirective(nil, home, "", config.TypeRole, name, rd); err != nil {
			t.Fatal(err)
		}
	}
	directives, err := config.LoadDirectivesFromFiles(home)
	if err != nil {
		t.Fatal(err)
	}
	kit.d.App.Directives = directives
}

func TestMenuCtxOmitsDeckAndConditionalRole(t *testing.T) {
	kit := testKit(t)

	kit.d.App.Cfg.Role = ""
	if got := kit.menuCtx(); len(got) != 0 {
		t.Errorf("menuCtx with no role = %v, want empty (no role, no deck)", got)
	}

	kit.d.App.Cfg.Role = "triage"
	got := kit.menuCtx()
	if len(got) != 1 || got[0].Key != "role" || got[0].Label != "triage" {
		t.Errorf("menuCtx = %v, want a single role=triage cue and no deck cue", got)
	}
}

func TestMainMenuIncludesNotesEntry(t *testing.T) {
	kit := testKit(t)
	kit.d.App.Cfg.Role = "triage"

	labels := make([]string, 0, len(kit.mainMenuItems()))
	for _, it := range kit.mainMenuItems() {
		labels = append(labels, it.Label)
	}
	found := false
	for _, l := range labels {
		if l == "Notes" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("main menu missing Notes entry: %v", labels)
	}

	app := newTestApp(kit.MainMenu())
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	body := app.View()
	if !strings.Contains(body, "Notes") {
		t.Fatalf("main menu view missing Notes: %q", body)
	}

	app = step(app, tea.KeyMsg{Type: tea.KeyDown})
	app, cmd := update(app, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		for _, c := range flattenCmds(cmd) {
			if c == nil {
				continue
			}
			app = step(app, c())
		}
	}
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	got := app.View()
	for _, want := range []string{"Notes", "Tasks"} {
		if !strings.Contains(got, want) {
			t.Fatalf("NTR home missing %q after Notes enter: %q", want, got)
		}
	}
	if strings.Contains(got, "Reminders") {
		t.Fatalf("NTR home showed Reminders without attached service: %q", got)
	}
}

func TestMainMenuDirectivesSubmenu(t *testing.T) {
	kit := testKit(t)

	mainLabels := make([]string, 0, len(kit.mainMenuItems()))
	for _, it := range kit.mainMenuItems() {
		mainLabels = append(mainLabels, it.Label)
	}
	if mainLabels[0] != "Directives" {
		t.Fatalf("main menu first item = %q, want Directives; got %v", mainLabels[0], mainLabels)
	}
	for _, banned := range []string{"Take flight", "Fly", "History", "Reports"} {
		for _, l := range mainLabels {
			if l == banned {
				t.Fatalf("main menu still has %q: %v", banned, mainLabels)
			}
		}
	}

	labels := make([]string, 0, len(kit.directiveMenuItems()))
	for _, it := range kit.directiveMenuItems() {
		labels = append(labels, it.Label)
	}
	want := []string{"Flights", "Queries", "Roles", "Reports"}
	if len(labels) != len(want) {
		t.Fatalf("directives submenu = %v, want %v (History needs an audit store)", labels, want)
	}
	for i, w := range want {
		if labels[i] != w {
			t.Fatalf("directives submenu[%d] = %q, want %q (full %v)", i, labels[i], w, labels)
		}
	}

	app := newTestApp(kit.MainMenu())
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	if !strings.Contains(app.View(), "Directives") {
		t.Fatalf("main menu view missing Directives: %q", app.View())
	}
	app, cmd := update(app, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		for _, c := range flattenCmds(cmd) {
			if c == nil {
				continue
			}
			app = step(app, c())
		}
	}
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	got := app.View()
	for _, want := range []string{"Flights", "Roles", "Reports"} {
		if !strings.Contains(got, want) {
			t.Fatalf("directives submenu view missing %q: %q", want, got)
		}
	}
}

func TestDirectivesSubmenuIncludesHistoryWhenPresent(t *testing.T) {
	st, err := audit.Open(context.Background(), filepath.Join(t.TempDir(), "audit.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	fid := st.StartFlight("default", "triage")
	st.FinishFlight(fid)

	kit := testKit(t)
	kit.d.App.Audit = st

	labels := make([]string, 0, len(kit.directiveMenuItems()))
	for _, it := range kit.directiveMenuItems() {
		labels = append(labels, it.Label)
	}
	want := []string{"Flights", "Queries", "Roles", "Reports", "History"}
	if len(labels) != len(want) {
		t.Fatalf("directives submenu = %v, want %v", labels, want)
	}
	for i, w := range want {
		if labels[i] != w {
			t.Fatalf("directives submenu[%d] = %q, want %q (full %v)", i, labels[i], w, labels)
		}
	}
	for _, it := range kit.mainMenuItems() {
		if it.Label == "History" || it.Label == "Roles" {
			t.Fatalf("main menu still exposes %q", it.Label)
		}
	}
}

func TestMainMenuToolingSubmenu(t *testing.T) {
	kit := testKit(t)

	mainLabels := make([]string, 0, len(kit.mainMenuItems()))
	for _, it := range kit.mainMenuItems() {
		mainLabels = append(mainLabels, it.Label)
	}
	foundTooling := false
	for _, l := range mainLabels {
		if l == "Tooling" {
			foundTooling = true
			break
		}
	}
	if !foundTooling {
		t.Fatalf("main menu missing Tooling: %v", mainLabels)
	}
	for _, banned := range []string{"Accounts", "Plugins", "Settings"} {
		for _, l := range mainLabels {
			if l == banned {
				t.Fatalf("main menu still has %q: %v", banned, mainLabels)
			}
		}
	}

	toolingLabels := make([]string, 0, len(kit.toolingMenuItems()))
	for _, it := range kit.toolingMenuItems() {
		toolingLabels = append(toolingLabels, it.Label)
	}
	want := []string{"Accounts", "Plugins", "Settings"}
	if len(toolingLabels) != len(want) {
		t.Fatalf("tooling submenu = %v, want %v", toolingLabels, want)
	}
	for i, w := range want {
		if toolingLabels[i] != w {
			t.Fatalf("tooling submenu[%d] = %q, want %q (full %v)", i, toolingLabels[i], w, toolingLabels)
		}
	}
	for _, it := range kit.toolingMenuItems() {
		if it.Label == "Plugins" && !strings.Contains(it.Desc, "install") {
			t.Fatalf("Plugins desc should mention install: %q", it.Desc)
		}
	}

	app := newTestApp(kit.MainMenu())
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	if !strings.Contains(app.View(), "Tooling") {
		t.Fatalf("main menu view missing Tooling: %q", app.View())
	}
	for range 3 {
		app = step(app, tea.KeyMsg{Type: tea.KeyDown})
	}
	app, cmd := update(app, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		for _, c := range flattenCmds(cmd) {
			if c == nil {
				continue
			}
			app = step(app, c())
		}
	}
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	got := app.View()
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Fatalf("tooling submenu view missing %q: %q", w, got)
		}
	}
}

func TestNTRHomeShowsRemindersWhenServiceAttached(t *testing.T) {
	pub.SetServiceAttachedFunc(func() bool { return true })
	t.Cleanup(func() { pub.SetServiceAttachedFunc(plugin.ServiceAttached) })

	kit := testKit(t)
	app := newTestApp(kit.NTR())
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	got := app.View()
	for _, want := range []string{"Notes", "Tasks", "Reminders"} {
		if !strings.Contains(got, want) {
			t.Fatalf("NTR home missing %q with attached service: %q", want, got)
		}
	}
	if item := kit.ntrMenuItem(); !strings.Contains(item.Desc, "reminders") {
		t.Fatalf("Notes menu desc = %q, want reminders mentioned", item.Desc)
	}
}

func TestViewsSmoke(t *testing.T) {
	kit := testKit(t)
	roots := map[string]vkdeck.View{
		"main":       kit.MainMenu(),
		"directives": kit.Directives(),
		"tooling":    kit.Tooling(),
		"queries":    kit.Queries(),
		"flights":    kit.Flights(),
		"flightnew":  kit.FlightBuilder(),
		"builder":    kit.QueryBuilder(),
		"roles":      kit.Roles(),
		"rolenew":    kit.RoleBuilder(),
		"reports":    kit.Reports(),
		"reportnew":  kit.ReportBuilder(),
		"settings":   kit.Settings(),
		"audit":      kit.AuditQuery(),
		"ntr":        kit.NTR(),
		"plugins":    kit.Plugins(),
	}
	for name, root := range roots {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("view %q panicked: %v", name, r)
				}
			}()
			app := newTestApp(root)
			app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
			_ = app.View()
			app = step(app, tea.KeyMsg{Type: tea.KeyDown})
			_ = app.View()
			app = step(app, tea.KeyMsg{Type: tea.KeyEnter})
			_ = app.View()
			app = step(app, tea.KeyMsg{Type: tea.KeyEsc})
			_ = app.View()
		}()
	}
}

func TestDirectivesMenuOffersRolesAndReports(t *testing.T) {
	kit := testKit(t)
	app := newTestApp(kit.Directives())
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	got := app.View()
	for _, want := range []string{"Flights", "Queries", "Roles", "Reports"} {
		if !strings.Contains(got, want) {
			t.Errorf("directives menu missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "Formatters") {
		t.Errorf("directives menu still offers Formatters beside Reports: %q", got)
	}
}

func TestReportsListNewFirstAndScopesToRole(t *testing.T) {
	kit := testKit(t)
	kit.d.App.Directives.Formatters = map[string]config.FormatterDef{
		"brief":   {Name: "brief", Title: "Brief", Template: "a"},
		"verbose": {Name: "verbose", Template: "b"},
	}
	kit.d.App.Directives.Roles = map[string]config.RoleDef{
		"triage": {Name: "triage", Flights: []string{"default"}, Formatters: []string{"brief"}},
	}

	kit.d.App.Cfg.Role = ""
	app := newTestApp(kit.Reports())
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	body := app.View()
	newAt, briefAt := strings.Index(body, "New"), strings.Index(body, "brief")
	if newAt < 0 || briefAt < 0 {
		t.Fatalf("report list missing entries: %q", body)
	}
	if newAt > briefAt {
		t.Errorf("New should come before saved reports:\n%s", body)
	}
	if !strings.Contains(body, "verbose") {
		t.Errorf("unscoped list should show every report: %q", body)
	}

	kit.d.App.Cfg.Role = "triage"
	scoped := newTestApp(kit.Reports())
	scoped = step(scoped, tea.WindowSizeMsg{Width: 100, Height: 40})
	if body := scoped.View(); strings.Contains(body, "verbose") {
		t.Errorf("role triage should hide verbose: %q", body)
	}
}

func TestReportEditorFields(t *testing.T) {
	kit := testKit(t)
	kit.d.App.Directives.Formatters = map[string]config.FormatterDef{
		"brief": {Name: "brief", Title: "Brief", Template: "line one\nline two"},
	}

	v, ok := kit.ReportEditor("brief").(*reportView)
	if !ok {
		t.Fatal("report view has the wrong type")
	}
	fields := v.editorFields(map[string]any{
		"name":     "brief",
		"title":    "Brief",
		"template": "line one\nline two",
	})
	if len(fields) != 4 {
		t.Fatalf("report fields = %d, want 4: %+v", len(fields), fields)
	}
	if fields[0].Key != "flight" || fields[0].Kind != forms.FieldSelect {
		t.Errorf("first field = %q (%v), want the flight select", fields[0].Key, fields[0].Kind)
	}
	if len(fields[0].Options) == 0 || fields[0].Options[0] != reportNoFlight {
		t.Errorf("flight options = %v, want %q first", fields[0].Options, reportNoFlight)
	}
	byKey := map[string]forms.Field{}
	for _, f := range fields {
		byKey[f.Key] = f
	}
	tmpl, ok := byKey["template"]
	if !ok {
		t.Fatalf("report form missing template field: %+v", fields)
	}
	if tmpl.Kind != forms.FieldMultiline {
		t.Errorf("template field kind = %v, want FieldMultiline", tmpl.Kind)
	}
	if tmpl.Text != "line one\nline two" {
		t.Errorf("template field text = %q, want the stored template", tmpl.Text)
	}
	if tmpl.Suggest != nil {
		t.Error("template field has a Suggester; template syntax must not be completed")
	}
	if byKey["name"].Text != "brief" || byKey["title"].Text != "Brief" {
		t.Errorf("report name/title = %q/%q, want brief/Brief", byKey["name"].Text, byKey["title"].Text)
	}
}

func TestReportEditorRejectsEmptyTemplate(t *testing.T) {
	kit := testKit(t)
	v, ok := kit.ReportBuilder().(*reportView)
	if !ok {
		t.Fatal("report view has the wrong type")
	}
	v.set(t, "name", "brief")
	v.set(t, "template", "")

	if _, err := v.editorValue(); err == nil {
		t.Fatal("a report with no template should fail")
	}

	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	step(app, tea.KeyMsg{Type: tea.KeyCtrlS})

	directives, err := config.LoadDirectivesFromFiles(kit.d.App.Cfg.Home)
	if err != nil {
		t.Fatal(err)
	}
	if len(directives.Formatters) != 0 {
		t.Fatalf("empty template wrote a report: %+v", directives.Formatters)
	}
}

func TestReportEditorSaveWritesTemplate(t *testing.T) {
	kit := testKit(t)
	v, ok := kit.ReportBuilder().(*reportView)
	if !ok {
		t.Fatal("report view has the wrong type")
	}
	v.set(t, "template", "line one\nline two")
	v.set(t, "title", "Brief")
	v.set(t, "name", "brief")

	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	step(app, tea.KeyMsg{Type: tea.KeyCtrlS})

	directives, err := config.LoadDirectivesFromFiles(kit.d.App.Cfg.Home)
	if err != nil {
		t.Fatal(err)
	}
	fd, ok := directives.Formatters["brief"]
	if !ok {
		t.Fatalf("report not written: %+v (status %q)", directives.Formatters, v.Status())
	}
	if fd.Title != "Brief" {
		t.Errorf("report title = %q, want Brief", fd.Title)
	}
	if fd.Template != "line one\nline two" {
		t.Errorf("report template = %q, want both lines", fd.Template)
	}
}

func TestHistorySelectShowsFlightResults(t *testing.T) {
	st, err := audit.Open(context.Background(), filepath.Join(t.TempDir(), "audit.duckdb"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	start := time.Now()
	fid := st.StartFlight("morning", "triage")
	st.RecordQuery(fid, "incidents", "triage", start, time.Now(), []signals.Section{{
		Signal: "github",
		Title:  "github",
		Items:  []signals.Item{{Kind: "pr", Title: "recall-me", URL: "https://x/1", Timestamp: start}},
	}})
	st.FinishFlight(fid)

	kit := testKit(t)
	kit.d.App.Audit = st

	view := kit.History()
	app := newTestApp(view)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	app = settle(app, view.Init())
	body := app.View()
	if !strings.Contains(body, "morning") {
		t.Fatalf("history list missing run: %q", body)
	}

	app, cmd := update(app, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		for _, c := range flattenCmds(cmd) {
			if c == nil {
				continue
			}
			app = step(app, c())
		}
	}
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	got := app.View()
	if !strings.Contains(got, "recall-me") {
		t.Fatalf("selected run results missing item: %q", got)
	}
	if !strings.Contains(got, "flight: morning") && !strings.Contains(got, "morning") {
		t.Fatalf("selected run results missing flight title: %q", got)
	}
}

func update(a *vkdeck.Model, msg tea.Msg) (*vkdeck.Model, tea.Cmd) {
	m, cmd := a.Update(msg)
	return m.(*vkdeck.Model), cmd
}

func flattenCmds(cmd tea.Cmd) []tea.Cmd {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		return batch
	}
	return []tea.Cmd{func() tea.Msg { return msg }}
}

func step(a *vkdeck.Model, msg tea.Msg) *vkdeck.Model {
	m, _ := a.Update(msg)
	return m.(*vkdeck.Model)
}

func TestFlightResultsRerunRefetches(t *testing.T) {
	runs := 0
	kit := testKit(t)
	kit.d.FetchFlightAudited = func(name string) []signals.Section {
		runs++
		return []signals.Section{{
			Signal: "github",
			Title:  name,
			Items:  []signals.Item{{Kind: "pr", Title: fmt.Sprintf("run-%d", runs), URL: "https://x/1"}},
		}}
	}

	view := kit.FlightResults("default")
	app := newTestApp(view)
	app = step(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	app = settle(app, view.Init())
	if got := app.View(); !strings.Contains(got, "run-1") {
		t.Fatalf("initial flight results missing run-1: %q", got)
	}
	if got := app.View(); !strings.Contains(got, "rerun") {
		t.Errorf("flight results footer missing rerun hint: %q", got)
	}

	app = settle(app, cmdOf(update(app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})))
	if runs != 2 {
		t.Fatalf("rerun key ran the flight %d times, want 2", runs)
	}
	if got := app.View(); !strings.Contains(got, "run-2") {
		t.Fatalf("rerun did not refresh results: %q", got)
	}

	app = settle(app, cmdOf(update(app, vkdeck.ReloadMsg{})))
	if runs != 3 {
		t.Fatalf("ReloadMsg ran the flight %d times, want 3", runs)
	}
	if got := app.View(); !strings.Contains(got, "run-3") {
		t.Fatalf("ReloadMsg did not refresh results: %q", got)
	}
}

func reportKit(t *testing.T) (*Kit, *int) {
	t.Helper()
	kit := testKit(t)
	renders := 0
	kit.d.FetchFlightQueries = func(flight string, names []string) []signals.Section {
		return []signals.Section{{Signal: "github", Title: flight}}
	}
	kit.d.RenderReport = func(fd config.FormatterDef, label string, sections []signals.Section) (string, error) {
		renders++
		return fmt.Sprintf("%s on %s render %d", fd.Template, label, renders), nil
	}
	return kit, &renders
}

func TestReportEditorRendersDraftOverSelectedFlight(t *testing.T) {
	kit, renders := reportKit(t)
	v, ok := kit.ReportBuilder().(*reportView)
	if !ok {
		t.Fatal("report view has the wrong type")
	}
	v.set(t, "template", "draft-template")

	if _, err := v.renderer(); err == nil {
		t.Fatal("rendering with no flight selected should fail")
	}

	v.selectFlight(t, "default")
	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	app = settle(app, cmdOf(update(app, tea.KeyMsg{Type: tea.KeyCtrlR})))
	app = step(app, tea.WindowSizeMsg{Width: 120, Height: 40})

	if *renders != 1 {
		t.Fatalf("rendered %d times, want 1 (status %q)", *renders, v.Status())
	}
	if got := app.View(); !strings.Contains(got, "draft-template on default render 1") {
		t.Fatalf("results missing the draft render: %q", got)
	}

	app = settle(app, cmdOf(update(app, vkdeck.ReloadMsg{})))
	if *renders != 2 {
		t.Fatalf("ReloadMsg rendered %d times, want 2", *renders)
	}
	if got := app.View(); !strings.Contains(got, "render 2") {
		t.Fatalf("ReloadMsg did not re-render: %q", got)
	}
}

func TestReportEditorCopyAndWriteNeedARenderFirst(t *testing.T) {
	kit, _ := reportKit(t)
	var copied string
	var savedFor string
	kit.d.CopyText = func(text string) error {
		copied = text
		return nil
	}
	kit.d.SaveReport = func(formatter, text string) (string, error) {
		savedFor = formatter
		return filepath.Join(kit.d.App.Cfg.Home, "reports", formatter+".md"), nil
	}

	v, ok := kit.ReportBuilder().(*reportView)
	if !ok {
		t.Fatal("report view has the wrong type")
	}
	v.set(t, "template", "draft-template")
	v.set(t, "name", "weekly")
	v.selectFlight(t, "default")

	if _, err := v.CopyOutput(); err == nil {
		t.Error("copy before a render should fail")
	}
	if _, err := v.WriteOutput(); err == nil {
		t.Error("write before a render should fail")
	}

	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	settle(app, cmdOf(update(app, tea.KeyMsg{Type: tea.KeyCtrlR})))

	summary, err := v.CopyOutput()
	if err != nil {
		t.Fatalf("CopyOutput after a render: %v", err)
	}
	if !strings.Contains(copied, "draft-template on default") {
		t.Errorf("copied %q, want the rendered report", copied)
	}
	if !strings.Contains(summary, "clipboard") {
		t.Errorf("copy summary = %q", summary)
	}
	if _, err := v.WriteOutput(); err != nil {
		t.Fatalf("WriteOutput after a render: %v", err)
	}
	if savedFor != "weekly" {
		t.Errorf("saved report under %q, want weekly", savedFor)
	}
}

func TestReportEditorCopyKeyIsWired(t *testing.T) {
	kit, _ := reportKit(t)
	copies := 0
	kit.d.CopyText = func(string) error {
		copies++
		return nil
	}

	v, ok := kit.ReportBuilder().(*reportView)
	if !ok {
		t.Fatal("report view has the wrong type")
	}
	v.set(t, "template", "draft-template")
	v.selectFlight(t, "default")

	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	if !strings.Contains(app.View(), "ctrl+g") {
		t.Errorf("report editor footer missing the copy key:\n%s", app.View())
	}
	app = settle(app, cmdOf(update(app, tea.KeyMsg{Type: tea.KeyCtrlR})))
	app = settle(app, cmdOf(update(app, tea.KeyMsg{Type: tea.KeyCtrlG})))

	if copies != 1 {
		t.Fatalf("ctrl+g copied %d times, want 1 (status %q)", copies, v.Status())
	}
	if got := app.View(); !strings.Contains(got, "clipboard") {
		t.Errorf("copy summary not shown after ctrl+g:\n%s", got)
	}
}

func TestQueryBuilderDoesNotOfferCopyOrWrite(t *testing.T) {
	kit := testKit(t)
	app := newTestApp(kit.QueryBuilder())
	app = step(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	got := app.View()
	for _, glyph := range []string{"ctrl+g", "ctrl+w"} {
		if strings.Contains(got, glyph) {
			t.Errorf("query builder offers %s but produces no report text:\n%s", glyph, got)
		}
	}
}

func TestHistoryRunDeleteDropsTheRun(t *testing.T) {
	st, err := audit.Open(context.Background(), filepath.Join(t.TempDir(), "audit.duckdb"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	start := time.Now()
	fid := st.StartFlight("morning", "triage")
	st.RecordQuery(fid, "incidents", "triage", start, time.Now(), []signals.Section{{
		Signal: "github",
		Title:  "github",
		Items:  []signals.Item{{Kind: "pr", Title: "drop-me", URL: "https://x/1", Timestamp: start}},
	}})
	st.FinishFlight(fid)

	kit := testKit(t)
	kit.d.App.Audit = st
	runs, err := st.RecentEntries(10)
	if err != nil || len(runs) != 1 {
		t.Fatalf("RecentEntries = %v, %v, want one run", runs, err)
	}

	view := kit.historyRun(runs[0])
	app := newTestApp(view)
	app = step(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	app = settle(app, view.Init())

	app = step(app, tea.KeyMsg{Type: tea.KeyCtrlX})
	if got := app.View(); !strings.Contains(got, "delete run #") {
		t.Fatalf("ctrl+x did not open the confirm dialog: %q", got)
	}
	app = step(app, tea.KeyMsg{Type: tea.KeyEsc})
	if left, err := st.RecentEntries(10); err != nil || len(left) != 1 {
		t.Fatalf("cancelling deleted the run: %v, %v", left, err)
	}

	app = step(app, tea.KeyMsg{Type: tea.KeyCtrlX})
	app = step(app, tea.KeyMsg{Type: tea.KeyLeft})
	settle(app, cmdOf(update(app, tea.KeyMsg{Type: tea.KeyEnter})))

	left, err := st.RecentEntries(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Fatalf("run survived the delete: %v", left)
	}
}

func TestHistoryListShowsEmptyStateAndReloads(t *testing.T) {
	st, err := audit.Open(context.Background(), filepath.Join(t.TempDir(), "audit.duckdb"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	kit := testKit(t)
	kit.d.App.Audit = st

	view := kit.History()
	app := newTestApp(view)
	app = step(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	app = settle(app, view.Init())
	if got := app.View(); !strings.Contains(got, "no recorded runs") {
		t.Fatalf("empty history missing its note: %q", got)
	}

	fid := st.StartFlight("morning", "triage")
	st.FinishFlight(fid)
	app = settle(app, cmdOf(update(app, vkdeck.ReloadMsg{})))
	if got := app.View(); !strings.Contains(got, "morning") {
		t.Fatalf("reload did not pick up the new run: %q", got)
	}
}

func TestHomeFlightLandsWhileAnotherViewIsOnTop(t *testing.T) {
	kit := testKit(t)
	kit.d.FetchHomeFlight = func(name string) []signals.Section {
		return []signals.Section{{Signal: "github", Title: "from " + name}}
	}
	kit.d.App.Directives.Roles = map[string]config.RoleDef{
		"triage": {Name: "triage", Home: "default", Flights: []string{"default"}},
	}
	kit.d.App.Cfg.Role = "triage"

	home := kit.Home()
	host := newTestApp(home)
	host = step(host, tea.WindowSizeMsg{Width: 120, Height: 40})
	slow := home.Init()

	host = settle(host, cmdOf(update(host, tea.KeyMsg{Type: tea.KeyEnter})))
	host = settle(host, slow)
	host = settle(host, cmdOf(update(host, tea.KeyMsg{Type: tea.KeyEsc})))

	got := host.View()
	if strings.Contains(got, "loading home flight") {
		t.Fatalf("home stuck on loading after the flight landed off-screen:\n%s", got)
	}
	if !strings.Contains(got, "from default") {
		t.Fatalf("home missing flight rows:\n%s", got)
	}
}
