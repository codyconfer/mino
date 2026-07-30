package views

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/viewkit/browser"
	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/app/pane"
	"github.com/codyconfer/munin/internal/keymap"
	"github.com/codyconfer/munin/internal/render"
	"github.com/codyconfer/munin/internal/signals"
)

type detailLoadedMsg struct {
	detail *signals.ItemDetail
	err    error
}

type DetailView struct {
	ref     render.ItemRef
	fetch   func(signal string, it signals.Item) (*signals.ItemDetail, error)
	open    func(url string) error
	detail  *signals.ItemDetail
	err     error
	loading bool
	scroll  layout.ScrollState
	total   int
	rows    int
}

func (k *Kit) Detail(ref render.ItemRef) vkdeck.View {
	return &DetailView{ref: ref, fetch: k.d.FetchDetail}
}

func (v *DetailView) Title() string { return render.ItemLabel(v.ref.Item) }

func (v *DetailView) Init() tea.Cmd {
	if v.fetch == nil {
		return nil
	}
	v.loading = true
	ref := v.ref
	fetch := v.fetch
	return func() tea.Msg {
		d, err := fetch(ref.Signal, ref.Item)
		return detailLoadedMsg{detail: d, err: err}
	}
}

func (v *DetailView) Update(h *vkdeck.Model, msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		v.rows = max(m.Height-detailChromeReserve, 1)
		v.scroll.Scroll(0, v.total, layout.ViewportContentRows(v.rows))
		return nil
	case detailLoadedMsg:
		v.detail, v.err, v.loading = m.detail, m.err, false
		return nil
	case tea.KeyMsg:
		return v.handleKey(h, m)
	}
	return nil
}

const detailChromeReserve = 7

func (v *DetailView) handleKey(h *vkdeck.Model, m tea.KeyMsg) tea.Cmd {
	act, ok := keymap.Detail().Action(m.String())
	if !ok {
		return nil
	}
	rows := layout.ViewportContentRows(v.rows)
	switch act {
	case keys.Up:
		v.scroll.Scroll(-1, v.total, rows)
	case keys.Down:
		v.scroll.Scroll(1, v.total, rows)
	case keys.PageUp:
		v.scroll.Scroll(-max(rows, 1), v.total, rows)
	case keys.PageDown:
		v.scroll.Scroll(max(rows, 1), v.total, rows)
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

func (v *DetailView) Body(width, height int) string {
	f := layout.ScreenFrame(width)
	body := render.DetailPanel(f, v.ref, v.detail)
	if v.err != nil {
		body = theme.Cur().Cant.Render(signals.Clean(v.err.Error())) + "\n" + body
	}
	v.total = layout.CountLines(body)
	v.rows = max(height, 1)
	return layout.Viewport(body, height, v.scroll.Offset)
}

func (v *DetailView) Hints() [][2]string {
	km := keymap.Detail()
	hints := [][2]string{
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

func (v *DetailView) Context() [][2]string {
	var cues [][2]string
	if scope := render.ItemScope(v.ref.Item); scope != "" {
		cues = append(cues, [2]string{"repo", scope})
	} else if v.ref.Signal != "" {
		cues = append(cues, [2]string{"signal", v.ref.Signal})
	}
	switch {
	case v.loading:
		cues = append(cues, [2]string{"detail", "loading…"})
	case v.err != nil:
		cues = append(cues, [2]string{"detail", "unavailable"})
	}
	if v.ref.Meta["cache"] == "stale" {
		cues = append(cues, [2]string{"cache", "stale " + signals.CleanLine(v.ref.Meta["age"])})
	}
	return cues
}
