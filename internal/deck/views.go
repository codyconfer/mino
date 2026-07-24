package deck

import (
	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/keys"

	"github.com/codyconfer/munin/internal/keymap"
)

// MenuItem is a navigable menu row (alias of viewkit/deck).
type MenuItem = vkdeck.MenuItem

// NewMenu builds a menu view (viewkit/deck) using the active key scheme.
func NewMenu(title string, ctx [][2]string, items ...MenuItem) *vkdeck.Menu {
	return vkdeck.NewMenu(title, ctx, items...)
}

// NewMessage builds a dismissible message view.
func NewMessage(title, body string, ctx [][2]string) *vkdeck.Message {
	return vkdeck.NewMessage(title, body, ctx)
}

// Content is a scrollable lazy-loaded view backed by viewkit/deck.Scroll.
type Content struct {
	inner *vkdeck.Scroll
}

// NewContent builds a Content view.
func NewContent(title string, ctx, hints [][2]string, load func() string) *Content {
	s := vkdeck.NewScroll(title, ctx, hints, load)
	s.IsCancel = func(key string) bool {
		act, ok := keymap.Menu().Action(key)
		return ok && act == keys.Cancel
	}
	return &Content{inner: s}
}

func (c *Content) Title() string                        { return c.inner.Title() }
func (c *Content) Context() [][2]string                 { return c.inner.Context() }
func (c *Content) Hints() [][2]string                   { return c.inner.Hints() }
func (c *Content) Init() tea.Cmd                        { return c.inner.Init() }
func (c *Content) Update(a *State, msg tea.Msg) tea.Cmd { return c.inner.Update(a, msg) }
func (c *Content) Body(width, height int) string        { return c.inner.Body(width, height) }

const chromeReserve = 7
