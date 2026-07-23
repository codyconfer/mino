package views

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/munin/internal/audit"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/filter"
	"github.com/codyconfer/munin/internal/tui"
	"github.com/codyconfer/sisyphus"
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
		Home:             func() string { return cfg.Home },
		Role:             func() string { return cfg.Role },
		Directives:       func() *config.Directives { return directives },
		Config:           func() *config.Config { return cfg },
		Mgr:              func() *sisyphus.Manager { return nil },
		Audit:            func() *audit.Store { return nil },
		VisibleFlights:   func() []string { return directives.FlightNames() },
		RunQuery:         func(string) string { return "" },
		RunFlight:        func(string) string { return "" },
		RunFlightAudited: func(string) string { return "" },
		Verify:           func(string) []Finding { return nil },
		ExportDirectives: func() ([]string, error) { return nil, nil },
	})
}

func TestViewsSmoke(t *testing.T) {
	kit := testKit(t)
	roots := map[string]tui.View{
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
			app := tui.New(root)
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

func step(a *tui.App, msg tea.Msg) *tui.App {
	m, _ := a.Update(msg)
	return m.(*tui.App)
}
