package deck

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/munin/internal/signals"
)

func homeItems() []MenuItem {
	return []MenuItem{
		{Label: "Run a flight", Desc: "aggregate saved queries"},
		{Label: "Quit", Do: func(*State) tea.Cmd { return tea.Quit }},
	}
}

func TestHomeMenuOnly(t *testing.T) {
	home := NewHome("home", nil, homeItems(), "", nil)
	app := New(home)
	app = drive(app, tea.WindowSizeMsg{Width: 100, Height: 40})

	view := app.View()
	for _, want := range []string{"MAIN MENU", "Run a flight", "Quit"} {
		if !strings.Contains(view, want) {
			t.Errorf("menu-only home missing %q\n%s", want, view)
		}
	}
	if strings.Contains(view, "home flight") {
		t.Error("menu-only home should not render a flight panel")
	}
	for _, h := range home.Hints() {
		if h[0] == "tab" {
			t.Error("menu-only home should not offer a tab/focus hint")
		}
	}
}

func TestHomeLoadsFlightAndTogglesFocus(t *testing.T) {
	sections := []signals.Section{{
		Signal: "github",
		Title:  "Open PRs",
		Items:  []signals.Item{{Title: "Fix onboarding attestation"}},
	}}
	home := NewHome("home", nil, homeItems(), "morning",
		func() []signals.Section { return sections })

	app := New(home)
	app = drive(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	if cmd := home.Init(); cmd != nil {
		app = drive(app, cmd())
	}

	view := app.View()
	for _, want := range []string{"MAIN MENU", "home flight · morning", "Open PRs", "Fix onboarding attestation"} {
		if !strings.Contains(view, want) {
			t.Errorf("home with flight missing %q\n%s", want, view)
		}
	}
	if home.focus != focusMenu {
		t.Fatalf("initial focus = %d, want menu", home.focus)
	}

	app = drive(app, tea.KeyMsg{Type: tea.KeyTab})
	if home.focus != focusFlight {
		t.Fatalf("after tab focus = %d, want flight", home.focus)
	}
	sawMenuHint := false
	for _, h := range home.Hints() {
		if h[0] == "tab" && h[1] == "menu" {
			sawMenuHint = true
		}
	}
	if !sawMenuHint {
		t.Errorf("flight-focused hints missing tab→menu: %v", home.Hints())
	}

	drive(app, tea.KeyMsg{Type: tea.KeyTab})
	if home.focus != focusMenu {
		t.Fatalf("after second tab focus = %d, want menu", home.focus)
	}
}

func TestHomeMenuNavigationRunsItem(t *testing.T) {
	ran := false
	items := []MenuItem{
		{Label: "First"},
		{Label: "Second", Do: func(*State) tea.Cmd { ran = true; return nil }},
	}
	home := NewHome("home", nil, items, "", nil)
	app := New(home)
	app = drive(app, tea.WindowSizeMsg{Width: 100, Height: 40})

	app = drive(app, key("down"))
	drive(app, key("enter"))
	if !ran {
		t.Error("expected menu item Do to run after down+enter")
	}
}
