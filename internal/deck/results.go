package deck

import (
	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/list"

	"github.com/codyconfer/munin/internal/render"
	"github.com/codyconfer/munin/internal/signals"
)

func NewResults(title string, ctx [][2]string, load func() []signals.Section) *vkdeck.ItemList {
	return vkdeck.NewItemList(title, ctx,
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
}
