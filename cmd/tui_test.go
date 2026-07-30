package cmd

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/deck"
)

func TestBuildViewsHistorySelectable(t *testing.T) {
	shared = &app.App{
		Cfg:        &config.Config{Home: t.TempDir()},
		Directives: &config.Directives{},
	}

	kit := buildViews()
	app := deck.New(kit.MainMenu())
	app, _ = update(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	_ = app.View()

	hist := kit.History()
	if hist == nil {
		t.Fatal("history view is nil")
	}
	app = deck.New(hist)
	app, cmd := update(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	if cmd != nil {
		for _, load := range collectCmds(cmd()) {
			if load != nil {
				_ = load()
			}
		}
	}
	_ = app.View()
}

func update(a *vkdeck.Model, msg tea.Msg) (*vkdeck.Model, tea.Cmd) {
	m, cmd := a.Update(msg)
	return m.(*vkdeck.Model), cmd
}

func collectCmds(msg tea.Msg) []tea.Cmd {
	if batch, ok := msg.(tea.BatchMsg); ok {
		return batch
	}
	return nil
}

func TestDeckFlightNameTracksRoleHome(t *testing.T) {
	orig := shared
	t.Cleanup(func() { shared = orig })

	for _, tc := range []struct {
		name string
		role config.RoleDef
		want string
	}{
		{"home wins over first flight", config.RoleDef{Name: "triage", Home: "triage-check", Flights: []string{"morning", "triage-check"}}, "triage-check"},
		{"unknown home falls back", config.RoleDef{Name: "triage", Home: "ghost", Flights: []string{"morning"}}, "morning"},
		{"no home falls back", config.RoleDef{Name: "triage", Flights: []string{"morning"}}, "morning"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shared = &app.App{
				Cfg: &config.Config{Home: t.TempDir(), Role: "triage"},
				Directives: &config.Directives{
					Flights: map[string]config.Flight{
						"morning":      {Name: "morning"},
						"triage-check": {Name: "triage-check"},
					},
					Roles: map[string]config.RoleDef{"triage": tc.role},
				},
			}
			if got := deckFlightName(); got != tc.want {
				t.Errorf("deckFlightName() = %q, want %q", got, tc.want)
			}
		})
	}
}
