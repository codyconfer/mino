package views

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/viewkit/browser"
	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/ui"

	"github.com/codyconfer/mino/internal/app/pane"
	"github.com/codyconfer/mino/internal/keymap"
	"github.com/codyconfer/mino/internal/render"
	"github.com/codyconfer/mino/internal/signals"
)

type detailLoadedMsg struct {
	detail *signals.ItemDetail
	err    error
}

type detailAnimationMsg struct{}
type detailPollMsg struct{ generation int }

const detailAnimationInterval = 80 * time.Millisecond

type DetailView struct {
	ref            render.ItemRef
	fetch          func(signal string, it signals.Item) (*signals.ItemDetail, error)
	open           func(url string) error
	detail         *signals.ItemDetail
	err            error
	loading        bool
	frame          int
	animating      bool
	pollInterval   time.Duration
	pollGeneration int
	scroll         vkdeck.ScrollBody
}

func (k *Kit) Detail(ref render.ItemRef) vkdeck.View {
	return &DetailView{ref: ref, fetch: k.d.FetchDetail, pollInterval: workflowPollInterval}
}

func (v *DetailView) Title() string { return render.ItemLabel(v.ref.Item) }

func (v *DetailView) ConsoleLoading() bool { return v.loading }

func (v *DetailView) Init() tea.Cmd {
	animate := v.animate()
	if v.fetch == nil {
		return animate
	}
	v.loading = true
	ref := v.ref
	fetch := v.fetch
	return tea.Batch(animate, func() tea.Msg {
		d, err := fetch(ref.Signal, ref.Item)
		return detailLoadedMsg{detail: d, err: err}
	})
}

func (v *DetailView) animFrame() int {
	if !v.animating {
		return -1
	}
	return v.frame
}

func (v *DetailView) animate() tea.Cmd {
	if v.animating || !render.DetailAnimates(v.ref, v.detail) {
		return nil
	}
	v.animating = true
	return detailAnimationTick()
}

func (v *DetailView) Update(h *vkdeck.Model, msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case detailLoadedMsg:
		v.detail, v.err, v.loading = m.detail, m.err, false
		v.settleWorkflowItem()
		v.pollGeneration++
		return tea.Batch(v.animate(), v.pollTick(v.pollGeneration))
	case detailAnimationMsg:
		if !render.DetailAnimates(v.ref, v.detail) {
			v.animating = false
			return nil
		}
		v.frame++
		return detailAnimationTick()
	case detailPollMsg:
		if m.generation != v.pollGeneration || v.fetch == nil || !render.DetailAnimates(v.ref, v.detail) {
			return nil
		}
		ref, fetch := v.ref, v.fetch
		return func() tea.Msg {
			d, err := fetch(ref.Signal, ref.Item)
			return detailLoadedMsg{detail: d, err: err}
		}
	case tea.KeyMsg:
		return v.handleKey(h, m)
	}
	return nil
}

func (v *DetailView) pollTick(generation int) tea.Cmd {
	if v.pollInterval <= 0 || v.fetch == nil || !render.DetailAnimates(v.ref, v.detail) {
		return nil
	}
	return tea.Tick(v.pollInterval, func(time.Time) tea.Msg { return detailPollMsg{generation: generation} })
}

func (v *DetailView) settleWorkflowItem() {
	if v.detail == nil || v.detail.Kind != "workflow" || !render.ItemInProgress(v.ref.Item) || render.DetailHasInProgress(v.detail) {
		return
	}
	hasWorkflow := false
	for _, section := range v.detail.Sections {
		if section.Meta["run_id"] != "" {
			hasWorkflow = true
			if len(section.Rows) == 0 {
				return
			}
			for _, row := range section.Rows {
				switch row[1] {
				case "", "in progress", "queued", "pending", "waiting", "requested", "action required":
					return
				}
			}
		}
	}
	if !hasWorkflow {
		return
	}
	meta := make(map[string]string, len(v.ref.Item.Meta))
	for key, value := range v.ref.Item.Meta {
		meta[key] = value
	}
	meta["status"] = "completed"
	v.ref.Item.Meta = meta
}

func detailAnimationTick() tea.Cmd {
	return tea.Tick(detailAnimationInterval, func(time.Time) tea.Msg { return detailAnimationMsg{} })
}

func (v *DetailView) handleKey(h *vkdeck.Model, m tea.KeyMsg) tea.Cmd {
	act, ok := keymap.Detail(modelScope(h).Keys).Action(m.String())
	if !ok {
		return nil
	}
	if v.scroll.Handle(act) {
		return nil
	}
	switch act {
	case keys.Open:
		return v.openItem()
	case keys.Cancel:
		return h.Pop()
	}
	return nil
}

func (v *DetailView) openItem() tea.Cmd {
	if v.open != nil {
		url := v.ref.Item.URL
		open := v.open
		return func() tea.Msg {
			_ = open(url)
			return nil
		}
	}
	return openURL(v.ref.Item.URL)
}

func openURL(url string) tea.Cmd {
	if url == "" {
		return nil
	}
	return func() tea.Msg {
		_ = browser.Open(url)
		return nil
	}
}

func (v *DetailView) Body(f layout.Frame) string {
	height := f.Height
	f = f.Screen()
	body := render.DetailPanelFrame(f, v.ref, v.detail, v.animFrame())
	if v.err != nil {
		body = f.Theme().Cant.Render(signals.Clean(v.err.Error())) + "\n" + body
	}
	return v.scroll.View(f, body, height)
}

func (v *DetailView) Hints(scope *ui.Scope) []keys.Hint {
	km := keymap.Detail(scope.Keys)
	hints := []keys.Hint{
		km.HintLabeled(keys.Up, "scroll"),
		km.HintLabeled(keys.PageUp, "page"),
	}
	if v.ref.Item.URL != "" {
		hints = append(hints, km.HintLabeled(keys.Open, "open"))
	}
	return hints
}

func (v *DetailView) PaneSnapshot() (pane.Snapshot, bool) {
	if v.detail == nil {
		return pane.Snapshot{}, false
	}
	item := v.ref.Item
	return pane.Snapshot{
		Kind:   pane.KindDetail,
		Title:  render.ItemLabel(v.ref.Item),
		Origin: v.ref.Signal,
		Signal: v.ref.Signal,
		Item:   &item,
		Meta:   v.ref.Meta,
		Detail: v.detail,
	}, true
}

func (v *DetailView) Context(scope *ui.Scope) []keys.Hint {
	var cues []keys.Hint
	if scope := render.ItemScope(v.ref.Item); scope != "" {
		cues = append(cues, keys.Hint{Key: "repo", Label: scope})
	} else if v.ref.Signal != "" {
		cues = append(cues, keys.Hint{Key: "signal", Label: v.ref.Signal})
	}
	switch {
	case v.loading:
		cues = append(cues, keys.Hint{Key: "detail", Label: "loading…"})
	case v.err != nil:
		cues = append(cues, keys.Hint{Key: "detail", Label: "unavailable"})
	}
	if v.ref.Meta["cache"] == "stale" {
		cues = append(cues, keys.Hint{Key: "cache", Label: "stale " + signals.CleanLine(v.ref.Meta["age"])})
	}
	return cues
}
