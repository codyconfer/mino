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

type SelectFunc func(h *vkdeck.Model, ref render.ItemRef) tea.Cmd

type resultsAnimationMsg struct{ generation int }

const resultsAnimationInterval = 80 * time.Millisecond

type Results struct {
	*vkdeck.ItemList
	sections   []signals.Section
	frame      int
	generation int
	width      int
	loaded     bool
}

func NewResults(title string, ctx []keys.Hint, load func() []signals.Section, onSelect SelectFunc) *Results {
	r := &Results{}
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
			r.sections = sections
			r.loaded = true
			return render.SectionItemsFrame(layout.ScreenFrame(width-listIndent), sections, r.frame)
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
	r.ItemList = vkdeck.NewItemList(spec)
	return r
}

func (r *Results) Init() tea.Cmd {
	r.generation++
	return tea.Batch(r.ItemList.Init(), resultsAnimationTick(r.generation))
}

func (r *Results) Update(h *vkdeck.Model, msg tea.Msg) tea.Cmd {
	if key, ok := msg.(tea.KeyMsg); ok {
		if action, bound := h.UI().Keys.MapFor(keys.Reload).Action(key.String()); bound && action == keys.Reload {
			r.loaded = false
			r.generation++
			return tea.Batch(r.ItemList.Update(h, msg), resultsAnimationTick(r.generation))
		}
	}
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		r.width = m.Width
		cmd := r.ItemList.Update(h, msg)
		if render.SectionsHaveInProgress(r.sections) {
			r.generation++
			return tea.Batch(cmd, resultsAnimationTick(r.generation))
		}
		return cmd
	case resultsAnimationMsg:
		if m.generation != r.generation {
			return nil
		}
		if !r.loaded {
			return resultsAnimationTick(r.generation)
		}
		if !render.SectionsHaveInProgress(r.sections) {
			return nil
		}
		r.frame++
		transient := r.width + 1
		if transient <= 1 {
			transient = 2
		}
		r.ItemList.Update(h, tea.WindowSizeMsg{Width: transient})
		r.ItemList.Update(h, tea.WindowSizeMsg{Width: r.width})
		return resultsAnimationTick(r.generation)
	case vkdeck.ReloadMsg:
		r.loaded = false
		r.generation++
		return tea.Batch(r.ItemList.Update(h, msg), resultsAnimationTick(r.generation))
	default:
		return r.ItemList.Update(h, msg)
	}
}

func resultsAnimationTick(generation int) tea.Cmd {
	return tea.Tick(resultsAnimationInterval, func(time.Time) tea.Msg {
		return resultsAnimationMsg{generation: generation}
	})
}
