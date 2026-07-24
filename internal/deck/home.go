package deck

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/list"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/keymap"
	"github.com/codyconfer/munin/internal/render"
	"github.com/codyconfer/munin/internal/signals"
)

const (
	focusMenu = iota
	focusFlight
)

type Home struct {
	title      string
	ctx        [][2]string
	items      []MenuItem
	cursor     int
	flightName string
	load       func() []signals.Section

	focus    int
	flight   list.Model
	width    int
	sections []signals.Section
	loaded   bool
}

func NewHome(title string, ctx [][2]string, items []MenuItem, flightName string, load func() []signals.Section) *Home {
	return &Home{
		title:      title,
		ctx:        ctx,
		items:      items,
		flightName: flightName,
		load:       load,
		flight:     list.New(),
	}
}

type homeFlightLoadedMsg struct{ sections []signals.Section }

func (h *Home) Title() string        { return h.title }
func (h *Home) Context() [][2]string { return h.ctx }

func (h *Home) Hints() [][2]string {
	if h.focus == focusFlight {
		return [][2]string{{"↑/↓", "move"}, {"enter", "open"}, {"pgup/pgdn", "page"}, {"tab", "menu"}}
	}
	hints := [][2]string{{"↑/↓", "move"}, {"enter", "open"}}
	if h.hasFlight() {
		hints = append(hints, [2]string{"tab", "flight"})
	}
	return hints
}

func (h *Home) hasFlight() bool { return h.flightName != "" && h.load != nil }

func (h *Home) Init() tea.Cmd {
	if !h.hasFlight() {
		return nil
	}
	return func() tea.Msg { return homeFlightLoadedMsg{sections: h.load()} }
}

func (h *Home) Update(a *State, msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		h.width = m.Width
		h.refresh()
		return nil
	case homeFlightLoadedMsg:
		h.sections, h.loaded = m.sections, true
		h.refresh()
		return nil
	case tea.KeyMsg:
		return h.handleKey(a, m)
	}
	return nil
}

func (h *Home) handleKey(a *State, m tea.KeyMsg) tea.Cmd {
	if h.hasFlight() && m.String() == "tab" {
		if h.focus == focusMenu {
			h.focus = focusFlight
		} else {
			h.focus = focusMenu
		}
		h.flight.SetFocused(h.focus == focusFlight)
		return nil
	}
	if h.focus == focusFlight {
		switch m.String() {
		case "pgup":
			h.flight.Scroll(-1)
			return nil
		case "pgdown":
			h.flight.Scroll(1)
			return nil
		}
		act, ok := keymap.Menu().Action(m.String())
		if !ok {
			return nil
		}
		switch act {
		case keys.Up:
			h.flight.Move(-1)
		case keys.Down:
			h.flight.Move(1)
		case keys.Confirm:
			return openSelected(&h.flight)
		case keys.Cancel:
			h.focus = focusMenu
			h.flight.SetFocused(false)
		}
		return nil
	}
	act, ok := keymap.Menu().Action(m.String())
	if !ok {
		return nil
	}
	switch act {
	case keys.Up:
		if h.cursor > 0 {
			h.cursor--
		}
	case keys.Down:
		if h.cursor < len(h.items)-1 {
			h.cursor++
		}
	case keys.Confirm:
		if len(h.items) > 0 && h.items[h.cursor].Do != nil {
			return h.items[h.cursor].Do(a)
		}
	case keys.Cancel:
		return a.Pop()
	}
	return nil
}

func (h *Home) refresh() {
	if !h.hasFlight() || h.width == 0 {
		return
	}
	th := theme.Cur()
	switch {
	case !h.loaded:
		h.flight.SetItems([]list.Item{{Block: th.Dim.Render("░▒▓ loading home flight…")}})
	case len(h.sections) == 0:
		h.flight.SetItems([]list.Item{{Block: th.Dim.Render("nothing to show")}})
	default:
		h.flight.SetItems(render.SectionItems(layout.ScreenFrame(h.width-listIndent), h.sections))
	}
}

func (h *Home) menuRows(f layout.Frame) []string {
	th := theme.Cur()
	rows := make([]string, len(h.items))
	for i, it := range h.items {
		cursor := "  "
		label := th.Val.Render(it.Label)
		switch {
		case i == h.cursor && h.focus == focusMenu:
			cursor = th.Key.Render("▸ ")
			label = th.Key.Render(it.Label)
		case i == h.cursor:
			cursor = th.Dim.Render("▸ ")
		}
		row := cursor + renderIcon(it.Icon, it.Hue) + label
		if it.Desc != "" {
			row = f.Spread(row, th.Dim.Render(it.Desc))
		}
		rows[i] = row
	}
	return rows
}

func (h *Home) Body(width, height int) string {
	f := layout.ScreenFrame(width)
	menuBox := render.TitledBox(f, h.focus == focusMenu, "MAIN MENU", h.menuRows(f)...)
	if !h.hasFlight() {
		return menuBox
	}
	th := theme.Cur()
	label := "◈ home flight · " + h.flightName
	if h.focus == focusFlight {
		label = th.Accent.Render(label)
	} else {
		label = th.Dim.Render(label)
	}
	h.flight.SetSize(width, max(height-layout.CountLines(menuBox)-2, 1))
	return menuBox + "\n\n" + label + "\n" + h.flight.View()
}
