package deck

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	vkdeck "github.com/codyconfer/viewkit/deck"

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

func loadedResults(t *testing.T, onSelect SelectFunc) (*vkdeck.Model, *vkdeck.ItemList) {
	t.Helper()
	lst := NewResults("flight: morning", nil,
		func() []signals.Section { return resultSections() }, onSelect)
	app := New(lst)
	app = drive(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	if cmd := lst.Init(); cmd != nil {
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
	for _, h := range withSelect.Hints() {
		labels[h.Label] = true
	}
	if !labels["details"] || !labels["open"] {
		t.Errorf("hints = %v, want both details and open", withSelect.Hints())
	}

	_, plain := loadedResults(t, nil)
	labels = map[string]bool{}
	for _, h := range plain.Hints() {
		labels[h.Label] = true
	}
	if labels["details"] {
		t.Errorf("hints = %v, should not promise details without an OnSelect", plain.Hints())
	}
	if !labels["open"] {
		t.Errorf("hints = %v, want confirm to still read as open", plain.Hints())
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
