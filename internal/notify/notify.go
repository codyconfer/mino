package notify

import (
	"fmt"
	"strings"

	"github.com/codyconfer/viewkit/layout"
	vnotify "github.com/codyconfer/viewkit/notify"
	"github.com/codyconfer/viewkit/panels"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/signals"
)

func FromEvent(ev signals.Event) (vnotify.Notification, bool) {
	sec := ev.Section
	if sec.Err != nil {
		return vnotify.Negative(ev.Source+": error", sec.ErrString()), true
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
	return vnotify.Note(tone(first), title, msg), true
}

func tone(it signals.Item) vnotify.Tone {
	switch strings.ToLower(it.Kind) {
	case "mention", "review-requested", "review_requested", "assigned", "alert", "incident":
		return vnotify.ToneWarning
	case "merged", "approved", "completed", "resolved", "success", "closed":
		return vnotify.TonePositive
	}
	return vnotify.ToneNeutral
}

func Render(n vnotify.Notification) string {
	return panels.NotificationToast(layout.NewFrame(theme.BodyWidth), n)
}
