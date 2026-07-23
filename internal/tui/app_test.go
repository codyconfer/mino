package tui

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/munin/internal/keymap"
)

func TestMain(m *testing.M) {
	keymap.Install()
	os.Exit(m.Run())
}

func drive(a *App, msg tea.Msg) *App {
	m, _ := a.Update(msg)
	return m.(*App)
}

func key(s string) tea.KeyMsg {
	if s == "enter" {
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	if s == "down" {
		return tea.KeyMsg{Type: tea.KeyDown}
	}
	if s == "esc" {
		return tea.KeyMsg{Type: tea.KeyEsc}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestAppRendersChromeAndMenu(t *testing.T) {
	menu := NewMenu("main menu", [][2]string{{"role", "triage"}},
		MenuItem{Label: "Alpha", Desc: "first"},
		MenuItem{Label: "Beta", Desc: "second", Do: func(a *App) tea.Cmd {
			return a.Push(NewMessage("beta screen", "hello from beta", nil))
		}},
	)
	app := New(menu)
	app = drive(app, tea.WindowSizeMsg{Width: 100, Height: 40})

	view := app.View()
	for _, want := range []string{"MUNIN", "ono-sendai", "MAIN MENU", "role", "triage", "Alpha", "Beta", "quit"} {
		if !strings.Contains(view, want) {
			t.Errorf("main menu frame missing %q\n---\n%s", want, view)
		}
	}
}

func TestAppNavigation(t *testing.T) {
	menu := NewMenu("main", nil,
		MenuItem{Label: "Alpha"},
		MenuItem{Label: "Beta", Do: func(a *App) tea.Cmd {
			return a.Push(NewMessage("beta screen", "body", nil))
		}},
	)
	app := New(menu)
	app = drive(app, tea.WindowSizeMsg{Width: 100, Height: 40})

	app = drive(app, key("down"))
	app = drive(app, key("enter"))
	if got := app.top().Title(); got != "beta screen" {
		t.Fatalf("after enter, top = %q, want beta screen", got)
	}
	if !strings.Contains(app.View(), "BETA SCREEN") {
		t.Errorf("pushed view frame missing title:\n%s", app.View())
	}

	app = drive(app, key("esc"))
	if got := app.top().Title(); got != "main" {
		t.Fatalf("after esc, top = %q, want main", got)
	}
}
