package views

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/audit"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/filter"
	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/signals"
	"github.com/codyconfer/munin/internal/testenv"
	pub "github.com/codyconfer/munin/plugin"
)

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
		FetchQuery:         func(string) []signals.Section { return nil },
		FetchFlightAudited: func(string) []signals.Section { return nil },
		FetchFlightQueries: func(string, []string) []signals.Section { return nil },
		Verify:             func(string) []Finding { return nil },
		ExportDirectives:   func() ([]string, error) { return nil, nil },
	})
}

func TestMenuCtxOmitsDeckAndConditionalRole(t *testing.T) {
	kit := testKit(t)

	kit.d.App.Cfg.Role = ""
	if got := kit.menuCtx(); len(got) != 0 {
		t.Errorf("menuCtx with no role = %v, want empty (no role, no deck)", got)
	}

	kit.d.App.Cfg.Role = "triage"
	got := kit.menuCtx()
	if len(got) != 1 || got[0][0] != "role" || got[0][1] != "triage" {
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

	app := deck.New(kit.MainMenu())
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

func TestMainMenuFlySubmenu(t *testing.T) {
	kit := testKit(t)

	mainLabels := make([]string, 0, len(kit.mainMenuItems()))
	for _, it := range kit.mainMenuItems() {
		mainLabels = append(mainLabels, it.Label)
	}
	if mainLabels[0] != "Fly" {
		t.Fatalf("main menu first item = %q, want Fly; got %v", mainLabels[0], mainLabels)
	}
	for _, banned := range []string{"Take flight", "History", "Directives"} {
		for _, l := range mainLabels {
			if l == banned {
				t.Fatalf("main menu still has %q: %v", banned, mainLabels)
			}
		}
	}

	flyLabels := make([]string, 0, len(kit.flyMenuItems()))
	for _, it := range kit.flyMenuItems() {
		flyLabels = append(flyLabels, it.Label)
	}
	for _, want := range []string{"Flights", "Directives"} {
		found := false
		for _, l := range flyLabels {
			if l == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("fly submenu missing %q: %v", want, flyLabels)
		}
	}
	for _, l := range flyLabels {
		if l == "History" {
			t.Fatalf("fly submenu showed History without audit store: %v", flyLabels)
		}
	}

	app := deck.New(kit.MainMenu())
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	if !strings.Contains(app.View(), "Fly") {
		t.Fatalf("main menu view missing Fly: %q", app.View())
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
	for _, want := range []string{"Flights", "Directives"} {
		if !strings.Contains(got, want) {
			t.Fatalf("fly submenu view missing %q: %q", want, got)
		}
	}
}

func TestFlySubmenuIncludesHistoryWhenPresent(t *testing.T) {
	st, err := audit.Open(context.Background(), filepath.Join(t.TempDir(), "audit.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	fid := st.StartFlight("default", "triage")
	st.FinishFlight(fid)

	kit := testKit(t)
	kit.d.App.Audit = st

	flyLabels := make([]string, 0, len(kit.flyMenuItems()))
	for _, it := range kit.flyMenuItems() {
		flyLabels = append(flyLabels, it.Label)
	}
	want := []string{"Flights", "History", "Queries", "Directives"}
	if len(flyLabels) != len(want) {
		t.Fatalf("fly submenu = %v, want %v", flyLabels, want)
	}
	for i, w := range want {
		if flyLabels[i] != w {
			t.Fatalf("fly submenu[%d] = %q, want %q (full %v)", i, flyLabels[i], w, flyLabels)
		}
	}
	for _, it := range kit.mainMenuItems() {
		if it.Label == "History" || it.Label == "Directives" {
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

	app := deck.New(kit.MainMenu())
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
	app := deck.New(kit.NTR())
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
		"fly":        kit.Fly(),
		"tooling":    kit.Tooling(),
		"directives": kit.directivesMenu(),
		"queries":    kit.Queries(),
		"flights":    kit.Flights(),
		"flightnew":  kit.FlightBuilder(),
		"builder":    kit.QueryBuilder(),
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
			app := deck.New(root)
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

	app := deck.New(kit.History())
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	body := app.View()
	if !strings.Contains(body, "morning") {
		t.Fatalf("history menu missing run: %q", body)
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
