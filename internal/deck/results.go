package deck

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/viewkit/browser"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/list"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/keymap"
	"github.com/codyconfer/munin/internal/render"
	"github.com/codyconfer/munin/internal/signals"
)

const listIndent = 2

type Results struct {
	title string
	ctx   [][2]string
	load  func() []signals.Section

	list     list.Model
	width    int
	height   int
	sections []signals.Section
	ready    bool
	loaded   bool
}

func NewResults(title string, ctx [][2]string, load func() []signals.Section) *Results {
	r := &Results{title: title, ctx: ctx, load: load, list: list.New()}
	r.list.SetFocused(true)
	return r
}

type resultsLoadedMsg struct{ sections []signals.Section }

func (r *Results) Title() string        { return r.title }
func (r *Results) Context() [][2]string { return r.ctx }
func (r *Results) Hints() [][2]string {
	return [][2]string{{"↑/↓", "move"}, {"enter", "open"}, {"pgup/pgdn", "page"}}
}

func (r *Results) Init() tea.Cmd {
	return func() tea.Msg { return resultsLoadedMsg{sections: r.load()} }
}

func (r *Results) Update(a *State, msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		r.width = m.Width
		r.height = max(m.Height-chromeReserve, 1)
		r.ready = true
		r.refresh()
		return nil
	case resultsLoadedMsg:
		r.sections, r.loaded = m.sections, true
		r.refresh()
		return nil
	case tea.KeyMsg:
		return r.handleKey(a, m)
	}
	return nil
}

func (r *Results) handleKey(a *State, m tea.KeyMsg) tea.Cmd {
	switch m.String() {
	case "pgup":
		r.list.Scroll(-r.height)
		return nil
	case "pgdown":
		r.list.Scroll(r.height)
		return nil
	}
	act, ok := keymap.Menu().Action(m.String())
	if !ok {
		return nil
	}
	switch act {
	case keys.Up:
		r.list.Move(-1)
	case keys.Down:
		r.list.Move(1)
	case keys.Confirm:
		return openSelected(&r.list)
	case keys.Cancel:
		return a.Pop()
	}
	return nil
}

func (r *Results) refresh() {
	if !r.ready {
		return
	}
	r.list.SetSize(r.width, r.height)
	if r.loaded {
		r.list.SetItems(render.SectionItems(layout.ScreenFrame(r.width-listIndent), r.sections))
	}
}

func (r *Results) Body(width, height int) string {
	if !r.loaded {
		return theme.Cur().Dim.Render("░▒▓ loading…")
	}
	return r.list.View()
}

func openSelected(l *list.Model) tea.Cmd {
	it, ok := l.Selected()
	if !ok || it.Key == "" {
		return nil
	}
	url := it.Key
	return func() tea.Msg {
		_ = browser.Open(url)
		return nil
	}
}
