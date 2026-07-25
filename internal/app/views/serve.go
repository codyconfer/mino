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
type servePruneMsg time.Time

type ServeView struct {
	flight string
	events <-chan signals.Event
	queue  *vnotify.Queue
	count  int
	last   string
	closed bool
}

func NewServeView(flight string, events <-chan signals.Event) *ServeView {
	return &ServeView{flight: flight, events: events, queue: vnotify.NewQueue(500)}
}

func (v *ServeView) Title() string { return "serve" }

func (v *ServeView) Init() tea.Cmd {
	return tea.Batch(v.waitEvent(), v.pruneTick())
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

func (v *ServeView) pruneTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return servePruneMsg(t) })
}

func (v *ServeView) Update(a *vkdeck.Model, msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case serveEventMsg:
		if n, show := mnotify.FromEvent(m.ev); show {
			now := time.Now()
			v.queue.PushFor(n, now, serveInboxTTL)
			v.count++
			v.last = now.Format("15:04:05")
		}
		return v.waitEvent()
	case serveClosedMsg:
		v.closed = true
		return nil
	case servePruneMsg:
		v.queue.Prune(time.Time(m))
		return v.pruneTick()
	case tea.KeyMsg:
		if act, ok := keymap.Menu().Action(m.String()); ok && act == keys.Cancel {
			return a.Pop()
		}
	}
	return nil
}

func (v *ServeView) Body(width, height int) string {
	f := layout.ScreenFrame(width)
	ns := v.queue.Snapshot()
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
