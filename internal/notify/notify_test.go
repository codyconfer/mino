package notify

import (
	"testing"

	vnotify "github.com/codyconfer/viewkit/notify"

	"github.com/codyconfer/munin/internal/signals"
)

func TestFromEventReminderAlert(t *testing.T) {
	ev := signals.Event{
		Source: "ntr",
		Section: signals.Section{
			Signal: "ntr",
			Title:  "reminders",
			Items: []signals.Item{{
				Kind:  "alert",
				Title: "standup in 5m",
				Meta:  map[string]string{"type": "reminder"},
			}},
		},
	}
	n, ok := FromEvent(ev)
	if !ok {
		t.Fatal("expected notification")
	}
	if n.Tone != vnotify.ToneWarning {
		t.Fatalf("tone = %v, want warning for reminder alert", n.Tone)
	}
	if n.Title != "reminders" || n.Message == "" {
		t.Fatalf("note = %+v", n)
	}
}

func TestFromEventEmptySkipped(t *testing.T) {
	if _, ok := FromEvent(signals.Event{Source: "ntr", Section: signals.Section{Signal: "ntr"}}); ok {
		t.Fatal("empty section should not notify")
	}
}
