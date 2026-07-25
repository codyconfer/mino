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

func TestAlreadyAuthed(t *testing.T) {
	n := AlreadyAuthed("GitHub")
	if n.Tone != vnotify.ToneNeutral {
		t.Fatalf("tone = %v, want neutral", n.Tone)
	}
	if n.Title != "accounts" || n.Message != "GitHub already authorized" {
		t.Fatalf("note = %+v", n)
	}
}

func TestPluginToggled(t *testing.T) {
	on := PluginToggled("munin.demo", true)
	if on.Tone != vnotify.TonePositive || on.Title != "plugins" || on.Message != "munin.demo enabled" {
		t.Fatalf("enabled toast = %+v", on)
	}
	off := PluginToggled("munin.demo", false)
	if off.Tone != vnotify.ToneNeutral || off.Message != "munin.demo disabled" {
		t.Fatalf("disabled toast = %+v", off)
	}
}

func TestPluginInstalled(t *testing.T) {
	n := PluginInstalled("munin.ntr", 2, 0)
	if n.Tone != vnotify.TonePositive || n.Title != "plugins" {
		t.Fatalf("toast = %+v", n)
	}
	if n.Message != "munin.ntr installed (wrote 2, skipped 0)" {
		t.Fatalf("message = %q", n.Message)
	}
}

func TestPluginUninstalled(t *testing.T) {
	n := PluginUninstalled("munin.ntr", 1, 1)
	if n.Tone != vnotify.ToneNeutral || n.Message != "munin.ntr uninstalled (removed 1, kept 1)" {
		t.Fatalf("toast = %+v", n)
	}
}
