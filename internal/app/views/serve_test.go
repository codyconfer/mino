package views

import (
	"strings"
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
			Items: []signals.Item{{
				Kind:  "message",
				Title: "hello",
				URL:   "https://example.test/demo/1",
			}},
		},
	}
	_ = v.Update(app, serveEventMsg{ev})
	if v.count != 1 {
		t.Fatalf("event not counted: count=%d", v.count)
	}
	if len(v.refs) != 1 {
		t.Fatalf("item with a URL should be recorded as a ref: refs=%d", len(v.refs))
	}
	if v.refs[0].Item.URL != "https://example.test/demo/1" || v.refs[0].Signal != "demo" {
		t.Fatalf("unexpected ref: %+v", v.refs[0])
	}
	ref, ok := v.selected()
	if !ok {
		t.Fatal("recorded ref should be selectable in the event list")
	}
	if ref.Item.URL != "https://example.test/demo/1" {
		t.Fatalf("selected wrong ref: %+v", ref)
	}
	body := v.Body(100, 30)
	if body == "" {
		t.Fatal("body with a live notification should render")
	}
	if !strings.Contains(body, "events · 1") {
		t.Fatalf("body should render the event list panel:\n%s", body)
	}
	if !strings.Contains(body, "hello") {
		t.Fatalf("body should render the item title:\n%s", body)
	}

	v.toast.Queue().Prune(time.Now().Add(time.Hour))
	if got := v.toast.Queue().Len(); got != 0 {
		t.Fatalf("far-future prune should drain the queue, got %d", got)
	}

	if v.Update(app, serveClosedMsg{}); !v.closed {
		t.Fatal("serveClosedMsg should mark the view closed")
	}
	if body := v.Body(100, 30); body == "" {
		t.Fatal("closed empty inbox should still render")
	}
}
