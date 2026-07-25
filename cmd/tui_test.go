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
