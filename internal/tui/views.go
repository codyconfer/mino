package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/keymap"
	"github.com/codyconfer/munin/internal/render"
)

type MenuItem struct {
	Label string
	Desc  string
	Do    func(a *App) tea.Cmd
}

type Menu struct {
	title  string
	items  []MenuItem
	cursor int
	ctx    [][2]string
}

func NewMenu(title string, ctx [][2]string, items ...MenuItem) *Menu {
	return &Menu{title: title, items: items, ctx: ctx}
}

func (m *Menu) Title() string        { return m.title }
func (m *Menu) Init() tea.Cmd        { return nil }
func (m *Menu) Context() [][2]string { return m.ctx }
func (m *Menu) Hints() [][2]string {
	return [][2]string{{"↑/↓", "move"}, {"enter", "open"}}
}

func (m *Menu) Update(a *App, msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	act, ok := keymap.Menu().Action(key.String())
	if !ok {
		return nil
	}
	switch act {
	case keys.Up:
		if m.cursor > 0 {
			m.cursor--
		}
	case keys.Down:
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case keys.Confirm:
		if len(m.items) > 0 && m.items[m.cursor].Do != nil {
			return m.items[m.cursor].Do(a)
		}
	case keys.Cancel:
		return a.Pop()
	}
	return nil
}

func (m *Menu) Body(width, _ int) string {
	th := theme.Cur()
	f := layout.ScreenFrame(width)
	var lines []string
	for i, it := range m.items {
		cursor := "  "
		label := th.Val.Render(it.Label)
		if i == m.cursor {
			cursor = th.Key.Render("▸ ")
			label = th.Key.Render(it.Label)
		}
		row := cursor + label
		if it.Desc != "" {
			row = f.Spread(row, th.Dim.Render(it.Desc))
		}
		lines = append(lines, row)
	}
	return render.TitledBox(f, true, strings.ToUpper(m.title), lines...)
}

type Content struct {
	title string
	load  func() string
	hints [][2]string
	ctx   [][2]string

	vp     viewport.Model
	ready  bool
	body   string
	loaded bool
}

func NewContent(title string, ctx, hints [][2]string, load func() string) *Content {
	return &Content{title: title, load: load, ctx: ctx, hints: hints}
}

type contentLoadedMsg struct{ body string }

func (c *Content) Title() string        { return c.title }
func (c *Content) Context() [][2]string { return c.ctx }
func (c *Content) Hints() [][2]string {
	return append([][2]string{{"↑/↓", "scroll"}, {"pgup/pgdn", "page"}}, c.hints...)
}

func (c *Content) Init() tea.Cmd {
	return func() tea.Msg { return contentLoadedMsg{body: c.load()} }
}

func (c *Content) Update(a *App, msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		h := max(m.Height-chromeReserve, 1)
		if !c.ready {
			c.vp = viewport.New(m.Width, h)
			c.ready = true
		} else {
			c.vp.Width, c.vp.Height = m.Width, h
		}
		c.refresh()
		return nil
	case contentLoadedMsg:
		c.body, c.loaded = m.body, true
		c.refresh()
		return nil
	case tea.KeyMsg:
		if act, ok := keymap.Menu().Action(m.String()); ok && act == keys.Cancel {
			return a.Pop()
		}
		if c.ready {
			var cmd tea.Cmd
			c.vp, cmd = c.vp.Update(msg)
			return cmd
		}
	}
	return nil
}

func (c *Content) refresh() {
	if !c.ready {
		return
	}
	if !c.loaded {
		c.vp.SetContent(theme.Cur().Dim.Render("░▒▓ loading…"))
		return
	}
	c.vp.SetContent(c.body)
}

func (c *Content) Body(width, height int) string {
	if !c.ready {
		return theme.Cur().Dim.Render("loading…")
	}
	return c.vp.View()
}

const chromeReserve = 7

type Message struct {
	title string
	body  string
	ctx   [][2]string
}

func NewMessage(title, body string, ctx [][2]string) *Message {
	return &Message{title: title, body: body, ctx: ctx}
}

func (m *Message) Title() string        { return m.title }
func (m *Message) Init() tea.Cmd        { return nil }
func (m *Message) Context() [][2]string { return m.ctx }
func (m *Message) Hints() [][2]string   { return nil }
func (m *Message) Update(a *App, msg tea.Msg) tea.Cmd {
	if key, ok := msg.(tea.KeyMsg); ok {
		if act, ok := keymap.Menu().Action(key.String()); ok && (act == keys.Cancel || act == keys.Confirm) {
			return a.Pop()
		}
	}
	return nil
}
func (m *Message) Body(width, _ int) string {
	f := layout.ScreenFrame(width)
	return render.TitledBox(f, true, strings.ToUpper(m.title), strings.Split(m.body, "\n")...)
}
