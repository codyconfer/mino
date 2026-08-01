package deck

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/mino/internal/signals"
)

func homeItems() []vkdeck.MenuItem {
	return []vkdeck.MenuItem{
		{Label: "Run a flight", Desc: "aggregate saved queries"},
		{Label: "Quit", OnSelect: func(*vkdeck.Model) tea.Cmd { return tea.Quit }},
	}
}

func TestHomeMenuOnly(t *testing.T) {
	home := NewHome("home", nil, homeItems(), nil, nil, nil)
	app := New(home)
	app = drive(app, tea.WindowSizeMsg{Width: 100, Height: 40})

	view := app.View()
	for _, want := range []string{"Run a flight", "Quit"} {
		if !strings.Contains(view, want) {
			t.Errorf("menu-only home missing %q\n%s", want, view)
		}
	}
	if strings.Contains(view, "MAIN MENU") {
		t.Errorf("menu-only home should omit MAIN MENU title\n%s", view)
	}
	if strings.Contains(view, "home flight") {
		t.Error("menu-only home should not render a flight panel")
	}
	for _, h := range home.Hints() {
		if h.Key == "tab" {
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
	home := NewHome("home", nil, homeItems(), func() string { return "morning" },
		func(string) []signals.Section { return sections }, nil)

	app := New(home)
	app = drive(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	if cmd := home.Init(); cmd != nil {
		app = drive(app, cmd())
	}

	view := app.View()
	for _, want := range []string{"home flight · morning", "Open PRs", "Fix onboarding attestation"} {
		if !strings.Contains(view, want) {
			t.Errorf("home with flight missing %q\n%s", want, view)
		}
	}
	if strings.Contains(view, "MAIN MENU") {
		t.Errorf("home with flight should omit MAIN MENU title\n%s", view)
	}
	if home.FocusSide() {
		t.Fatalf("initial focus on side, want menu")
	}

	app = drive(app, tea.KeyMsg{Type: tea.KeyTab})
	if !home.FocusSide() {
		t.Fatalf("after tab focus on menu, want flight")
	}
	sawMenuHint := false
	for _, h := range home.Hints() {
		if strings.HasPrefix(h.Key, "tab") && h.Label == "menu" {
			sawMenuHint = true
		}
	}
	if !sawMenuHint {
		t.Errorf("flight-focused hints missing tab→menu: %v", home.Hints())
	}

	drive(app, tea.KeyMsg{Type: tea.KeyTab})
	if home.FocusSide() {
		t.Fatalf("after second tab focus on side, want menu")
	}
}

func TestHomeMenuNavigationRunsItem(t *testing.T) {
	ran := false
	items := []vkdeck.MenuItem{
		{Label: "First"},
		{Label: "Second", OnSelect: func(*vkdeck.Model) tea.Cmd { ran = true; return nil }},
	}
	home := NewHome("home", nil, items, nil, nil, nil)
	app := New(home)
	app = drive(app, tea.WindowSizeMsg{Width: 100, Height: 40})

	app = drive(app, key("down"))
	drive(app, key("enter"))
	if !ran {
		t.Error("expected menu item Do to run after down+enter")
	}
}
