package deck

import (
	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/list"

	"github.com/codyconfer/munin/internal/render"
	"github.com/codyconfer/munin/internal/signals"
)

type SelectFunc func(h *vkdeck.Model, ref render.ItemRef) tea.Cmd

func NewResults(title, root string, ctx [][2]string, load func() []signals.Section, onSelect SelectFunc) *vkdeck.ItemList {
	index := map[string]render.ItemRef{}
	lst := vkdeck.NewItemList(title, ctx,
		func() any {
			if load == nil {
				return []signals.Section(nil)
			}
			return load()
		},
		func(width int, fetched any) []list.Item {
			sections, _ := fetched.([]signals.Section)
			index = render.ItemIndex(sections)
			return render.SectionItems(layout.ScreenFrame(width-listIndent), root, sections)
		},
	)
	if onSelect != nil {
		lst.OnSelect = func(h *vkdeck.Model, key string) tea.Cmd {
			ref, ok := index[key]
			if !ok {
				return nil
			}
			return onSelect(h, ref)
		}
	}
	return lst
}
