//go:build !nodaemon

package views

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/signals"
)

func TestServeViewRendersAndPrunes(t *testing.T) {
	ch := make(chan signals.Event)
	v := NewServeView("watch", ch)
	app := deck.New(v)
	m, _ := app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	app = m.(*vkdeck.Model)

	ev := signals.Event{
		Source: "demo",
		At:     time.Now(),
		Section: signals.Section{
			Signal: "demo",
			Title:  "demo",
			Items:  []signals.Item{{Kind: "message", Title: "hello"}},
		},
	}
	_ = v.Update(app, serveEventMsg{ev})
	if v.count != 1 {
		t.Fatalf("event not counted: count=%d", v.count)
	}
	if body := v.Body(100, 30); body == "" {
		t.Fatal("body with a live notification should render")
	}

	_ = v.Update(app, servePruneMsg(time.Now().Add(time.Hour)))
	if got := v.queue.Len(); got != 0 {
		t.Fatalf("far-future prune should drain the queue, got %d", got)
	}

	if v.Update(app, serveClosedMsg{}); !v.closed {
		t.Fatal("serveClosedMsg should mark the view closed")
	}
	if body := v.Body(100, 30); body == "" {
		t.Fatal("closed empty inbox should still render")
	}
}
