package serve

import (
	"testing"

	"github.com/codyconfer/sisyphus/tray"

	"github.com/codyconfer/mino/internal/notify"
	"github.com/codyconfer/mino/internal/signals"
)

func TestReminderEventDrivesNotifySinkContract(t *testing.T) {
	ev := signals.Event{
		Source: "ntr",
		Section: signals.Section{
			Signal: "ntr",
			Title:  "reminders",
			Items:  []signals.Item{{Kind: "alert", Title: "pay invoice"}},
		},
	}
	if st := stateForEvent(ev); st != tray.StateWarn {
		t.Fatalf("state = %v, want warn", st)
	}
	n, ok := notify.FromEvent(ev)
	if !ok || n.Message == "" {
		t.Fatalf("FromEvent = %+v ok=%v", n, ok)
	}
}
