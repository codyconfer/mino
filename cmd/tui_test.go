package cmd

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/deck"
)

func TestBuildViewsHistoryLoads(t *testing.T) {
	shared = &app.App{
		Cfg:        &config.Config{Home: t.TempDir()},
		Directives: &config.Directives{},
	}

	kit := buildViews()
	app := deck.New(kit.MainMenu())

	app, _ = update(app, tea.KeyMsg{Type: tea.KeyDown})
	_, cmd := update(app, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("opening History returned no command")
	}

	for _, load := range collectCmds(cmd()) {
		if load != nil {
			_ = load()
		}
	}
}

func update(a *deck.State, msg tea.Msg) (*deck.State, tea.Cmd) {
	m, cmd := a.Update(msg)
	return m.(*deck.State), cmd
}

func collectCmds(msg tea.Msg) []tea.Cmd {
	if batch, ok := msg.(tea.BatchMsg); ok {
		return batch
	}
	return nil
}
