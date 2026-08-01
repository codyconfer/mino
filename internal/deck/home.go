package deck

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/list"

	"github.com/codyconfer/mino/internal/render"
	"github.com/codyconfer/mino/internal/signals"
)

const listIndent = 2

type homeAnimationMsg struct{ generation int }

const homeAnimationInterval = 80 * time.Millisecond

type Home struct {
	*vkdeck.HomeShell

	flightName func() string
	sections   []signals.Section
	frame      int
	generation int
	width      int
	loaded     bool
}

func NewHome(title string, ctx []keys.Hint, items []vkdeck.MenuItem, flightName func() string, load func(name string) []signals.Section, onSelect SelectFunc) *Home {
	h := &Home{flightName: flightName}
	spec := vkdeck.HomeShellSpec{
		Title:       title,
		Ctx:         ctx,
		Items:       items,
		SideHint:    "flight",
		SideLoading: "░▒▓ loading home flight…",
		ReloadHint:  "rerun flight",
	}
	if flightName == nil || load == nil {
		h.HomeShell = vkdeck.NewHomeShell(spec)
		return h
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
		f := layout.ScreenFrame(width - listIndent)
		sections, _ := fetched.([]signals.Section)
		h.sections, h.loaded = sections, true
		if len(sections) == 0 {
			return []list.Item{{Block: f.Theme().Dim.Italic(true).Render("nothing to show")}}
		}
		return render.SectionItemsFrame(f, sections, h.frame)
	}
	if onSelect != nil {
		spec.OnSelect = func(m *vkdeck.Model, it list.Item) tea.Cmd {
			ref, ok := it.Payload.(render.ItemRef)
			if !ok {
				return nil
			}
			return onSelect(m, ref)
		}
	}
	h.HomeShell = vkdeck.NewHomeShell(spec)
	return h
}

func (h *Home) animates() bool { return h.flightName != nil }

func (h *Home) Init() tea.Cmd {
	cmd := h.HomeShell.Init()
	if !h.animates() {
		return cmd
	}
	h.generation++
	return tea.Batch(cmd, homeAnimationTick(h.generation))
}

func (h *Home) Update(m *vkdeck.Model, msg tea.Msg) tea.Cmd {
	switch t := msg.(type) {
	case tea.WindowSizeMsg:
		h.width = t.Width
		cmd := h.HomeShell.Update(m, msg)
		if !h.animates() || !render.SectionsHaveInProgress(h.sections) {
			return cmd
		}
		h.generation++
		return tea.Batch(cmd, homeAnimationTick(h.generation))
	case homeAnimationMsg:
		return h.advance(m, t.generation)
	case vkdeck.ReloadMsg:
		return h.restart(h.HomeShell.Update(m, msg))
	case tea.KeyMsg:
		cmd := h.HomeShell.Update(m, msg)
		if act, bound := m.UI().Keys.MapFor(keys.Reload).Action(t.String()); bound && act == keys.Reload {
			return h.restart(cmd)
		}
		return cmd
	}
	return h.HomeShell.Update(m, msg)
}

func (h *Home) advance(m *vkdeck.Model, generation int) tea.Cmd {
	if generation != h.generation {
		return nil
	}
	if !h.loaded {
		if h.flightName == nil || h.flightName() == "" {
			return nil
		}
		return homeAnimationTick(h.generation)
	}
	if !render.SectionsHaveInProgress(h.sections) {
		return nil
	}
	h.frame++
	h.rebind(m)
	return homeAnimationTick(h.generation)
}

func (h *Home) restart(cmd tea.Cmd) tea.Cmd {
	if !h.animates() {
		return cmd
	}
	h.loaded = false
	h.generation++
	return tea.Batch(cmd, homeAnimationTick(h.generation))
}

func (h *Home) rebind(m *vkdeck.Model) {
	transient := h.width + 1
	if transient <= 1 {
		transient = 2
	}
	h.HomeShell.Update(m, tea.WindowSizeMsg{Width: transient})
	h.HomeShell.Update(m, tea.WindowSizeMsg{Width: h.width})
}

func homeAnimationTick(generation int) tea.Cmd {
	return tea.Tick(homeAnimationInterval, func(time.Time) tea.Msg {
		return homeAnimationMsg{generation: generation}
	})
}
