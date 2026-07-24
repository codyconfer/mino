package views

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/filter"
	"github.com/codyconfer/munin/internal/signals"
)

func testKit(t *testing.T) *Kit {
	t.Helper()
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

func TestViewsSmoke(t *testing.T) {
	kit := testKit(t)
	roots := map[string]deck.View{
		"main":       kit.MainMenu(),
		"directives": kit.directivesMenu(),
		"settings":   kit.Settings(),
		"audit":      kit.AuditQuery(),
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

func step(a *deck.State, msg tea.Msg) *deck.State {
	m, _ := a.Update(msg)
	return m.(*deck.State)
}
