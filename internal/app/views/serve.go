//go:build !nodaemon

package views

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	vnotify "github.com/codyconfer/viewkit/notify"
	"github.com/codyconfer/viewkit/panels"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/munin/internal/keymap"
	mnotify "github.com/codyconfer/munin/internal/notify"
	"github.com/codyconfer/munin/internal/signals"
)

const serveInboxTTL = 5 * time.Minute

type serveEventMsg struct{ ev signals.Event }
type serveClosedMsg struct{}
type ServeView struct {
	flight string
	events <-chan signals.Event
	toast  *vkdeck.Toaster
	count  int
	last   string
	closed bool
}

func NewServeView(flight string, events <-chan signals.Event) *ServeView {
	return &ServeView{flight: flight, events: events, toast: vkdeck.NewToaster(500, serveInboxTTL)}
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
	case serveEventMsg:
		var tick tea.Cmd
		if n, show := mnotify.FromEvent(m.ev); show {
			tick = v.toast.PushFor(n, serveInboxTTL)
			v.count++
			v.last = time.Now().Format("15:04:05")
		}
		return tea.Batch(tick, v.waitEvent())
	case serveClosedMsg:
		v.closed = true
		return nil
	case tea.KeyMsg:
		if act, ok := keymap.Menu().Action(m.String()); ok && act == keys.Cancel {
			return a.Pop()
		}
	}
	return nil
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
	return panels.NotificationPanel(f, title, recent)
}

func (v *ServeView) Hints() [][2]string { return [][2]string{{"esc", "back"}} }

func (v *ServeView) Context() [][2]string {
	cues := [][2]string{{"flight", v.flight}}
	if v.last != "" {
		cues = append(cues, [2]string{"last", v.last})
	}
	return cues
}
