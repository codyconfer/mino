package notify

import (
	"fmt"

	"github.com/codyconfer/viewkit/glyph"
	"github.com/codyconfer/viewkit/layout"
	vnotify "github.com/codyconfer/viewkit/notify"
	"github.com/codyconfer/viewkit/panels"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/signals"
)

func FromEvent(ev signals.Event) (vnotify.Notification, bool) {
	sec := ev.Section
	if sec.Err != nil {
		return vnotify.Negative(ev.Source+": error", signals.Clean(sec.ErrString())), true
	}
	if len(sec.Items) == 0 {
		return vnotify.Notification{}, false
	}
	title := sec.Title
	if title == "" {
		title = ev.Source
	}
	first := sec.Items[0]
	msg := first.Title
	if first.Subtitle != "" {
		msg = first.Subtitle + " · " + msg
	}
	if n := len(sec.Items); n > 1 {
		msg = fmt.Sprintf("%s (+%d more)", msg, n-1)
	}
	return vnotify.Note(toneFor(signals.ClassifyKind(first.Kind)), signals.Clean(title), signals.Clean(msg)), true
}

func toneFor(sev glyph.Severity) vnotify.Tone {
	switch sev {
	case glyph.SeverityWarning:
		return vnotify.ToneWarning
	case glyph.SeverityPositive:
		return vnotify.TonePositive
	case glyph.SeverityNegative:
		return vnotify.ToneNegative
	default:
		return vnotify.ToneNeutral
	}
}

func Render(n vnotify.Notification) string {
	return panels.NotificationToast(layout.NewFrame(theme.BodyWidth), n)
}

func AlreadyAuthed(label string) vnotify.Notification {
	return vnotify.Neutral("accounts", label+" already authorized")
}

func PluginToggled(id string, on bool) vnotify.Notification {
	if on {
		return vnotify.Positive("plugins", id+" enabled")
	}
	return vnotify.Neutral("plugins", id+" disabled")
}

func PluginInstalled(id string, written, skipped int) vnotify.Notification {
	return vnotify.Positive("plugins", fmt.Sprintf("%s installed (wrote %d, skipped %d)", id, written, skipped))
}

func PluginUninstalled(id string, removed, kept int) vnotify.Notification {
	return vnotify.Neutral("plugins", fmt.Sprintf("%s uninstalled (removed %d, kept %d)", id, removed, kept))
}
