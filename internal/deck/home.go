package deck

import (
	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/list"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/keymap"
	"github.com/codyconfer/munin/internal/render"
	"github.com/codyconfer/munin/internal/signals"
)

// Home is the main menu (+ optional home-flight side list) backed by HomeShell.
type Home struct {
	inner *vkdeck.HomeShell
}

// NewHome builds a Home view. flightName/load enable the side pane.
func NewHome(title string, ctx [][2]string, items []MenuItem, flightName string, load func() []signals.Section) *Home {
	label := ""
	if flightName != "" && load != nil {
		label = "home flight · " + flightName
	}
	shell := vkdeck.NewHomeShell(title, ctx, items, label)
	shell.SideHint = "flight"
	shell.SideLoading = "░▒▓ loading home flight…"
	shell.IsAction = keymap.Menu().Action
	if label != "" {
		shell.SideFetch = func() any { return load() }
		shell.SideBind = func(width int, fetched any) []list.Item {
			th := theme.Cur()
			sections, _ := fetched.([]signals.Section)
			if len(sections) == 0 {
				return []list.Item{{Block: th.Dim.Render("nothing to show")}}
			}
			return render.SectionItems(layout.ScreenFrame(width-listIndent), sections)
		}
	}
	return &Home{inner: shell}
}

func (h *Home) Title() string                        { return h.inner.Title() }
func (h *Home) Context() [][2]string                 { return h.inner.Context() }
func (h *Home) Hints() [][2]string                   { return h.inner.Hints() }
func (h *Home) Init() tea.Cmd                        { return h.inner.Init() }
func (h *Home) Update(a *State, msg tea.Msg) tea.Cmd { return h.inner.Update(a, msg) }
func (h *Home) Body(width, height int) string        { return h.inner.Body(width, height) }

// focusSide reports side-pane focus (tests).
func (h *Home) focusSide() bool { return h.inner.FocusSide() }
