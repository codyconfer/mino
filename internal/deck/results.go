package deck

import (
	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/list"

	"github.com/codyconfer/munin/internal/keymap"
	"github.com/codyconfer/munin/internal/render"
	"github.com/codyconfer/munin/internal/signals"
)

const listIndent = 2

// Results is a flight/section list view backed by viewkit/deck.ItemList.
type Results struct {
	inner *vkdeck.ItemList
}

// NewResults builds a Results view that maps signals.Section → list rows.
func NewResults(title string, ctx [][2]string, load func() []signals.Section) *Results {
	il := vkdeck.NewItemList(title, ctx,
		func() any {
			if load == nil {
				return []signals.Section(nil)
			}
			return load()
		},
		func(width int, fetched any) []list.Item {
			sections, _ := fetched.([]signals.Section)
			return render.SectionItems(layout.ScreenFrame(width-listIndent), sections)
		},
	)
	il.ChromeReserve = chromeReserve
	il.IsAction = keymap.Menu().Action
	il.IsCancel = func(key string) bool {
		act, ok := keymap.Menu().Action(key)
		return ok && act == keys.Cancel
	}
	return &Results{inner: il}
}

func (r *Results) Title() string                        { return r.inner.Title() }
func (r *Results) Context() [][2]string                 { return r.inner.Context() }
func (r *Results) Hints() [][2]string                   { return r.inner.Hints() }
func (r *Results) Init() tea.Cmd                        { return r.inner.Init() }
func (r *Results) Update(a *State, msg tea.Msg) tea.Cmd { return r.inner.Update(a, msg) }
func (r *Results) Body(width, height int) string        { return r.inner.Body(width, height) }
