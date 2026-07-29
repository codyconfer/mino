package deck

import (
	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/list"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/render"
	"github.com/codyconfer/munin/internal/signals"
)

const listIndent = 2

func NewHome(title string, ctx [][2]string, items []vkdeck.MenuItem, flightName string, load func() []signals.Section, onSelect SelectFunc) *vkdeck.HomeShell {
	label := ""
	if flightName != "" && load != nil {
		label = "home flight · " + flightName
	}
	shell := vkdeck.NewHomeShell(title, ctx, items, label)
	shell.SideHint = "flight"
	shell.SideLoading = "░▒▓ loading home flight…"
	if label != "" {
		index := map[string]render.ItemRef{}
		shell.SideFetch = func() any { return load() }
		shell.SideBind = func(width int, fetched any) []list.Item {
			th := theme.Cur()
			sections, _ := fetched.([]signals.Section)
			index = render.ItemIndex(sections)
			if len(sections) == 0 {
				return []list.Item{{Block: th.Dim.Render("nothing to show")}}
			}
			return render.SectionItems(layout.ScreenFrame(width-listIndent), flightName, sections)
		}
		if onSelect != nil {
			shell.OnSelect = func(h *vkdeck.Model, key string) tea.Cmd {
				ref, ok := index[key]
				if !ok {
					return nil
				}
				return onSelect(h, ref)
			}
		}
	}
	return shell
}
