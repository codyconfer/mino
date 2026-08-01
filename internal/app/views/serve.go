package views

import (
	"fmt"
	"slices"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/list"
	vnotify "github.com/codyconfer/viewkit/notify"
	"github.com/codyconfer/viewkit/panels"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/mino/internal/keymap"
	mnotify "github.com/codyconfer/mino/internal/notify"
	"github.com/codyconfer/mino/internal/render"
	"github.com/codyconfer/mino/internal/signals"
)

const (
	serveInboxTTL      = 5 * time.Minute
	serveRingCap       = 200
	serveChromeReserve = 5
)

type serveEventMsg struct{ ev signals.Event }
type serveClosedMsg struct{}

type ServeView struct {
	flight string
	events <-chan signals.Event
	toast  *vkdeck.Toaster
	count  int
	last   string
	closed bool

	FetchDetail func(signal string, it signals.Item) (*signals.ItemDetail, error)

	refs   []render.ItemRef
	lst    list.Model
	width  int
	height int
}

func NewServeView(flight string, events <-chan signals.Event) *ServeView {
	v := &ServeView{flight: flight, events: events, toast: vkdeck.NewToaster(500, serveInboxTTL), lst: list.New()}
	v.lst.SetFocused(true)
	return v
}

func (v *ServeView) record(ev signals.Event) {
	for _, it := range ev.Section.Items {
		if it.URL == "" {
			continue
		}
		v.refs = append(v.refs, render.ItemRef{Signal: ev.Source, Item: it, Meta: ev.Section.Meta})
	}
	if len(v.refs) > serveRingCap {
		v.refs = v.refs[len(v.refs)-serveRingCap:]
	}
	v.rebind()
}

func (v *ServeView) rebind() {
	if v.width == 0 {
		return
	}
	items := make([]signals.Item, 0, len(v.refs))
	for _, r := range v.refs {
		items = append(items, r.Item)
	}
	v.lst.SetItemsKeepingCursor(render.ItemRows(layout.ScreenFrame(v.width), items))
}

func (v *ServeView) selected() (render.ItemRef, bool) {
	it, ok := v.lst.Selected()
	if !ok || it.Key == "" {
		return render.ItemRef{}, false
	}
	for _, r := range slices.Backward(v.refs) {
		if r.Item.URL == it.Key {
			return r, true
		}
	}
	return render.ItemRef{}, false
}

func (v *ServeView) Title() string { return "serve" }

func (v *ServeView) Init() tea.Cmd {
	return v.waitEvent()
}

func (v *ServeView) waitEvent() tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-v.events
		if !ok {
			return serveClosedMsg{}
		}
		return serveEventMsg{ev}
	}
}

func (v *ServeView) Update(a *vkdeck.Model, msg tea.Msg) tea.Cmd {
	if cmd, handled := v.toast.Update(msg); handled {
		return cmd
	}
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		if m.Width == v.width && m.Height == v.height {
			return nil
		}
		v.width, v.height = m.Width, m.Height
		v.lst.SetSize(m.Width, max(m.Height/2-serveChromeReserve, 1))
		v.rebind()
		return nil
	case serveEventMsg:
		var tick tea.Cmd
		if n, show := mnotify.FromEvent(m.ev); show {
			tick = v.toast.PushFor(n, serveInboxTTL)
			v.count++
			v.last = time.Now().Format("15:04:05")
		}
		v.record(m.ev)
		return tea.Batch(tick, v.waitEvent())
	case serveClosedMsg:
		v.closed = true
		return nil
	case tea.KeyMsg:
		return v.handleKey(a, m)
	}
	return nil
}

func (v *ServeView) handleKey(a *vkdeck.Model, m tea.KeyMsg) tea.Cmd {
	act, ok := keymap.ItemList().Action(m.String())
	if !ok {
		return nil
	}
	switch act {
	case keys.Up:
		v.lst.Move(-1)
	case keys.Down:
		v.lst.Move(1)
	case keys.PageUp:
		v.lst.Scroll(-max(v.lst.Height(), 1))
	case keys.PageDown:
		v.lst.Scroll(max(v.lst.Height(), 1))
	case keys.Confirm:
		return v.confirm(a)
	case keys.Open:
		if ref, ok := v.selected(); ok {
			return openURL(ref.Item.URL)
		}
	case keys.Cancel:
		return a.Pop()
	}
	return nil
}

func (v *ServeView) confirm(a *vkdeck.Model) tea.Cmd {
	ref, ok := v.selected()
	if !ok || v.FetchDetail == nil {
		return nil
	}
	return a.Push(&DetailView{ref: ref, fetch: v.FetchDetail})
}

func (v *ServeView) Body(width, height int) string {
	f := layout.ScreenFrame(width)
	v.toast.Prune()
	ns := v.toast.Queue().Snapshot()
	recent := make([]vnotify.Notification, 0, len(ns))
	for i := len(ns) - 1; i >= 0; i-- {
		recent = append(recent, ns[i])
	}
	max := height / 2
	if max < 1 {
		max = 1
	}
	if len(recent) > max {
		recent = recent[:max]
	}
	title := fmt.Sprintf("inbox · %d live · %d total", len(ns), v.count)
	if v.closed {
		title += " · stream closed"
	}
	inbox := panels.NotificationPanel(f, title, recent)
	if len(v.refs) == 0 {
		return inbox
	}
	return layout.Stack(inbox, f.Panel(fmt.Sprintf("events · %d", len(v.refs)), v.lst.View()))
}

func (v *ServeView) Hints() [][2]string {
	km := keymap.ItemList()
	hints := [][2]string{km.HintLabeled(keys.Up, "move")}
	if v.FetchDetail != nil {
		hints = append(hints, km.HintLabeled(keys.Confirm, "details"))
	}
	return append(hints, km.HintLabeled(keys.Open, "open"))
}

func (v *ServeView) Context() [][2]string {
	cues := [][2]string{{"flight", v.flight}}
	if v.last != "" {
		cues = append(cues, [2]string{"last", v.last})
	}
	return cues
}
