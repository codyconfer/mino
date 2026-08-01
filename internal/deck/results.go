package deck

import (
	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/list"

	"github.com/codyconfer/mino/internal/render"
	"github.com/codyconfer/mino/internal/signals"
)

type SelectFunc func(h *vkdeck.Model, ref render.ItemRef) tea.Cmd

func NewResults(title string, ctx []keys.Hint, load func() []signals.Section, onSelect SelectFunc) *vkdeck.ItemList {
	spec := vkdeck.ItemListSpec{
		Title: title,
		Ctx:   ctx,
		Fetch: func() any {
			if load == nil {
				return []signals.Section(nil)
			}
			return load()
		},
		Bind: func(width int, fetched any) []list.Item {
			sections, _ := fetched.([]signals.Section)
			return render.SectionItems(layout.ScreenFrame(width-listIndent), sections)
		},
		ReloadHint: "rerun",
	}
	if onSelect != nil {
		spec.OnSelect = func(h *vkdeck.Model, it list.Item) tea.Cmd {
			ref, ok := it.Payload.(render.ItemRef)
			if !ok {
				return nil
			}
			return onSelect(h, ref)
		}
	}
	return vkdeck.NewItemList(spec)
}
