package daemon

import (
	"testing"

	sysdaemon "github.com/codyconfer/sisyphus/daemon"

	"github.com/codyconfer/munin/internal/notify"
	"github.com/codyconfer/munin/internal/signals"
)

// Reminder events from NTR Scheduled must drive tray warn + a visible note
// for desktop/terminal sinks (M4 notify fan-in contract).
func TestReminderEventDrivesNotifySinkContract(t *testing.T) {
	ev := signals.Event{
		Source: "ntr",
		Section: signals.Section{
			Signal: "ntr",
			Title:  "reminders",
			Items:  []signals.Item{{Kind: "alert", Title: "pay invoice"}},
		},
	}
	if st := stateForEvent(ev); st != sysdaemon.StateWarn {
		t.Fatalf("state = %v, want warn", st)
	}
	n, ok := notify.FromEvent(ev)
	if !ok || n.Message == "" {
		t.Fatalf("FromEvent = %+v ok=%v", n, ok)
	}
}
