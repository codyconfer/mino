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
	"github.com/codyconfer/munin/internal/signals"
	"github.com/codyconfer/munin/internal/testenv"
)

func testKit(t *testing.T) *Kit {
	t.Helper()
	testenv.Isolate(t)
	cfg := config.Defaults()
	cfg.Home = t.TempDir()
	directives := &config.Directives{
		Queries: map[string]config.Query{"q1": {Name: "q1", Signal: "github"}},
		Filters: map[string]filter.Filter{"f1": {Name: "f1"}},
		Flights: map[string]config.Flight{"default": {Name: "default", Queries: []string{"q1"}}},
		Roles:   map[string]config.RoleDef{"triage": {Name: "triage", Flights: []string{"default"}}},
	}
	return New(Deps{
		App:                &app.App{Cfg: cfg, Directives: directives},
		FetchQuery:         func(string) []signals.Section { return nil },
		FetchFlight:        func(string) []signals.Section { return nil },
		FetchFlightAudited: func(string) []signals.Section { return nil },
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
	for _, want := range []string{"Notes", "Tasks", "Reminders"} {
		if !strings.Contains(got, want) {
			t.Fatalf("NTR home missing %q after Notes enter: %q", want, got)
		}
	}
}

func TestViewsSmoke(t *testing.T) {
	kit := testKit(t)
	roots := map[string]vkdeck.View{
		"main":       kit.MainMenu(),
		"directives": kit.directivesMenu(),
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
