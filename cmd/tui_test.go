package cmd

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/tui"
)

func TestBuildViewsHistoryLoads(t *testing.T) {
	shared.cfg = &config.Config{Home: t.TempDir()}
	shared.directives = &config.Directives{}
	shared.audit = nil

	kit := buildViews()
	app := tui.New(kit.MainMenu())

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

func update(a *tui.App, msg tea.Msg) (*tui.App, tea.Cmd) {
	m, cmd := a.Update(msg)
	return m.(*tui.App), cmd
}

func collectCmds(msg tea.Msg) []tea.Cmd {
	if batch, ok := msg.(tea.BatchMsg); ok {
		return batch
	}
	return nil
}
