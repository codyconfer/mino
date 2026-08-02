package deck

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/ui"

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
	for _, h := range home.Hints(ui.Default()) {
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
	app = run(app, home.Init())

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
	for _, h := range home.Hints(ui.Default()) {
		if strings.HasPrefix(h.Key, "tab") && h.Label == "menu" {
			sawMenuHint = true
		}
	}
	if !sawMenuHint {
		t.Errorf("flight-focused hints missing tab→menu: %v", home.Hints(ui.Default()))
	}

	drive(app, tea.KeyMsg{Type: tea.KeyTab})
	if home.FocusSide() {
		t.Fatalf("after second tab focus on side, want menu")
	}
}

func TestHomeAnimatesInProgressWorkflow(t *testing.T) {
	sections := []signals.Section{{
		Signal: "github",
		Title:  "Workflows",
		Items: []signals.Item{{
			Kind:  "workflow",
			Title: "CI #42",
			URL:   "https://github.com/acme/tools/actions/runs/42",
			Meta:  map[string]string{"status": "in_progress"},
		}},
	}}
	home := NewHome("home", nil, homeItems(), func() string { return "morning" },
		func(string) []signals.Section { return sections }, nil)

	app := New(home)
	app = drive(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	app = run(app, home.Init())

	first := ansi.Strip(app.View())
	if cmd := home.Update(app, homeAnimationMsg{generation: home.generation}); cmd == nil {
		t.Fatal("in-progress workflow did not continue the animation tick")
	}
	second := ansi.Strip(app.View())
	if first == second {
		t.Fatalf("home flight spinner stayed frozen:\n%s", second)
	}
}

func TestHomeStopsAnimatingSettledFlight(t *testing.T) {
	sections := []signals.Section{{
		Signal: "github",
		Title:  "Workflows",
		Items: []signals.Item{{
			Kind:  "workflow",
			Title: "CI #41",
			Meta:  map[string]string{"status": "completed", "conclusion": "success"},
		}},
	}}
	home := NewHome("home", nil, homeItems(), func() string { return "morning" },
		func(string) []signals.Section { return sections }, nil)

	app := New(home)
	app = drive(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	app = run(app, home.Init())

	if cmd := home.Update(app, homeAnimationMsg{generation: home.generation}); cmd != nil {
		t.Fatal("settled flight kept scheduling animation ticks")
	}
}

func TestHomeRerunsFlightWhenBackNavigationReturns(t *testing.T) {
	loads := 0
	sections := []signals.Section{{
		Signal: "github",
		Title:  "Open PRs",
		Items:  []signals.Item{{Title: "Fix onboarding attestation"}},
	}}
	home := NewHome("home", nil, homeItems(), func() string { return "morning" },
		func(string) []signals.Section {
			loads++
			return sections
		}, nil)

	app := New(home)
	app = drive(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	app = run(app, home.Init())
	if loads != 1 {
		t.Fatalf("loads after the initial render = %d, want 1", loads)
	}

	app = run(app, app.Push(vkdeck.NewMessage("detail", "body", nil)))
	app = run(app, app.Pop())
	if loads != 2 {
		t.Fatalf("loads after returning home = %d, want 2", loads)
	}
	if !home.loaded {
		t.Error("home flight never finished its rerun")
	}
	if !strings.Contains(app.View(), "Fix onboarding attestation") {
		t.Errorf("reloaded home lost its flight items\n%s", app.View())
	}
}

func TestHomeWithoutFlightIgnoresBackNavigation(t *testing.T) {
	home := NewHome("home", nil, homeItems(), nil, nil, nil)
	app := New(home)
	app = drive(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	app = run(app, app.Push(vkdeck.NewMessage("detail", "body", nil)))
	if cmd := home.Resume(app); cmd != nil {
		t.Error("menu-only home scheduled a rerun on back navigation")
	}
}

func TestHomeAnimationWaitsForLoad(t *testing.T) {
	home := NewHome("home", nil, homeItems(), func() string { return "morning" },
		func(string) []signals.Section { return nil }, nil)
	home.generation = 1
	if cmd := home.Update(New(home), homeAnimationMsg{generation: 1}); cmd == nil {
		t.Fatal("animation tick stopped before the home flight loaded")
	}
}

func TestHomeWithoutFlightStopsTicking(t *testing.T) {
	home := NewHome("home", nil, homeItems(), func() string { return "" },
		func(string) []signals.Section { return nil }, nil)
	home.generation = 1
	if cmd := home.Update(New(home), homeAnimationMsg{generation: 1}); cmd != nil {
		t.Fatal("roleless home kept an animation timer alive")
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
