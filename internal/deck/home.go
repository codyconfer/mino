package deck

import (
	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/list"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/mino/internal/render"
	"github.com/codyconfer/mino/internal/signals"
)

const listIndent = 2

func NewHome(title string, ctx []keys.Hint, items []vkdeck.MenuItem, flightName func() string, load func(name string) []signals.Section, onSelect SelectFunc) *vkdeck.HomeShell {
	spec := vkdeck.HomeShellSpec{
		Title:       title,
		Ctx:         ctx,
		Items:       items,
		SideHint:    "flight",
		SideLoading: "░▒▓ loading home flight…",
		ReloadHint:  "rerun flight",
	}
	if flightName == nil || load == nil {
		return vkdeck.NewHomeShell(spec)
	}
	spec.SideLabelFn = func() string {
		if name := flightName(); name != "" {
			return "home flight · " + name
		}
		return ""
	}
	spec.SideFetch = func() any {
		name := flightName()
		if name == "" {
			return []signals.Section(nil)
		}
		return load(name)
	}
	spec.SideBind = func(width int, fetched any) []list.Item {
		th := theme.Cur()
		sections, _ := fetched.([]signals.Section)
		if len(sections) == 0 {
			return []list.Item{{Block: th.Dim.Render("nothing to show")}}
		}
		return render.SectionItems(layout.ScreenFrame(width-listIndent), sections)
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
	return vkdeck.NewHomeShell(spec)
}
