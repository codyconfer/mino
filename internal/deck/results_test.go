package deck

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/ui"

	"github.com/codyconfer/mino/internal/render"
	"github.com/codyconfer/mino/internal/signals"
)

func resultSections() []signals.Section {
	return []signals.Section{{
		Signal: "github",
		Title:  "Open PRs",
		Items: []signals.Item{
			{Kind: "pr", Title: "Fix backoff", URL: "https://github.com/acme/tools/pull/412"},
			{Kind: "pr", Title: "Bump viewkit", URL: "https://github.com/acme/tools/pull/401"},
		},
	}}
}

func loadedResults(t *testing.T, onSelect SelectFunc) (*vkdeck.Model, *Results) {
	t.Helper()
	lst := NewResults("flight: morning", nil,
		func() []signals.Section { return resultSections() }, onSelect)
	app := New(lst)
	app = drive(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	if cmd := lst.ItemList.Init(); cmd != nil {
		app = drive(app, cmd())
	}
	return app, lst
}

func TestResultsConfirmSelectsTheDomainItem(t *testing.T) {
	var got render.ItemRef
	app, _ := loadedResults(t, func(_ *vkdeck.Model, ref render.ItemRef) tea.Cmd {
		got = ref
		return nil
	})

	if _, cmd := app.Update(key("enter")); cmd != nil {
		cmd()
	}
	if got.Item.URL != "https://github.com/acme/tools/pull/412" {
		t.Fatalf("selected ref = %+v, want the first row's item", got)
	}
	if got.Signal != "github" {
		t.Errorf("ref signal = %q, want github", got.Signal)
	}
	if got.Item.Title != "Fix backoff" {
		t.Errorf("ref title = %q", got.Item.Title)
	}
}

func TestResultsConfirmFollowsTheCursor(t *testing.T) {
	var got render.ItemRef
	app, _ := loadedResults(t, func(_ *vkdeck.Model, ref render.ItemRef) tea.Cmd {
		got = ref
		return nil
	})

	app = drive(app, key("down"))
	if _, cmd := app.Update(key("enter")); cmd != nil {
		cmd()
	}
	if got.Item.URL != "https://github.com/acme/tools/pull/401" {
		t.Errorf("selected ref = %+v, want the second row", got)
	}
}

func TestResultsOpenGoesToTheBrowserHook(t *testing.T) {
	var opened string
	_, lst := loadedResults(t, func(*vkdeck.Model, render.ItemRef) tea.Cmd { return nil })
	lst.OnOpen = func(url string) error {
		opened = url
		return nil
	}

	app := New(lst)
	app = drive(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	if _, cmd := app.Update(key("o")); cmd != nil {
		cmd()
	}
	if opened != "https://github.com/acme/tools/pull/412" {
		t.Errorf("opened %q, want the selected URL", opened)
	}
}

func TestResultsHintsAdvertiseDetailsWhenSelectable(t *testing.T) {
	_, withSelect := loadedResults(t, func(*vkdeck.Model, render.ItemRef) tea.Cmd { return nil })
	labels := map[string]bool{}
	for _, h := range withSelect.Hints(ui.Default()) {
		labels[h.Label] = true
	}
	if !labels["details"] || !labels["open"] {
		t.Errorf("hints = %v, want both details and open", withSelect.Hints(ui.Default()))
	}

	_, plain := loadedResults(t, nil)
	labels = map[string]bool{}
	for _, h := range plain.Hints(ui.Default()) {
		labels[h.Label] = true
	}
	if labels["details"] {
		t.Errorf("hints = %v, should not promise details without an OnSelect", plain.Hints(ui.Default()))
	}
	if !labels["open"] {
		t.Errorf("hints = %v, want confirm to still read as open", plain.Hints(ui.Default()))
	}
}

func TestResultsAnimateInProgressWorkflow(t *testing.T) {
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
	lst := NewResults("workflows", nil, func() []signals.Section { return sections }, nil)
	app := New(lst)
	app = drive(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	app = drive(app, lst.ItemList.Init()())

	first := ansi.Strip(app.View())
	if cmd := lst.Update(app, resultsAnimationMsg{generation: lst.generation}); cmd == nil {
		t.Fatal("in-progress workflow did not continue the animation tick")
	}
	second := ansi.Strip(app.View())
	if first == second {
		t.Fatalf("workflow spinner stayed frozen:\n%s", second)
	}
}

func TestResultsAnimationWaitsForLoad(t *testing.T) {
	lst := NewResults("workflows", nil, nil, nil)
	lst.generation = 1
	if cmd := lst.Update(New(lst), resultsAnimationMsg{generation: 1}); cmd == nil {
		t.Fatal("animation tick stopped before results loaded")
	}
}

func TestResultsPollReloadsOnlyInProgressResults(t *testing.T) {
	sections := []signals.Section{{Items: []signals.Item{{
		Kind: "workflow", Meta: map[string]string{"status": "in_progress"},
	}}}}
	lst := NewResults("workflows", nil, func() []signals.Section { return sections }, nil).PollEvery(time.Minute)
	lst.sections, lst.loaded, lst.generation = sections, true, 4
	if cmd := lst.Update(New(lst), resultsPollMsg{generation: 4}); cmd == nil {
		t.Fatal("in-progress results did not reload on a poll")
	}
	if lst.loaded || lst.generation != 5 {
		t.Fatalf("poll state = loaded %t, generation %d; want false, 5", lst.loaded, lst.generation)
	}

	lst.sections[0].Items[0].Meta["status"] = "completed"
	lst.loaded = true
	if cmd := lst.Update(New(lst), resultsPollMsg{generation: 5}); cmd != nil {
		t.Fatal("settled results continued polling")
	}
}

func TestResultsConfirmPushesAView(t *testing.T) {
	app, _ := loadedResults(t, func(a *vkdeck.Model, ref render.ItemRef) tea.Cmd {
		return a.Push(vkdeck.NewMessage(render.ItemLabel(ref.Item), "detail body", nil))
	})

	app, cmd := update(app, key("enter"))
	if cmd != nil {
		app = drive(app, cmd())
	}
	out := ansi.Strip(app.View())
	if !strings.Contains(out, "pr #412") {
		t.Errorf("want the pushed view's breadcrumb\n%s", out)
	}
	if !strings.Contains(out, "detail body") {
		t.Errorf("want the pushed view's body\n%s", out)
	}
}

func update(a *vkdeck.Model, msg tea.Msg) (*vkdeck.Model, tea.Cmd) {
	m, cmd := a.Update(msg)
	return m.(*vkdeck.Model), cmd
}
