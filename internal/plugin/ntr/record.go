package ntr

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/codyconfer/viewkit/theme"
	"github.com/codyconfer/viewkit/timefmt"

	"github.com/codyconfer/mino/internal/render/glyph"
)

const (
	kindNote     = "note"
	kindTask     = "task"
	kindReminder = "reminder"
)

type record struct {
	Kind  string
	ID    int64
	Title string
	Body  string
	Due   time.Time
	Done  bool
}

type recordYAML struct {
	Kind  string `yaml:"kind"`
	ID    int64  `yaml:"id,omitempty"`
	Title string `yaml:"title"`
	Body  string `yaml:"body,omitempty"`
	Due   string `yaml:"due,omitempty"`
	Done  bool   `yaml:"done,omitempty"`
}

func (r record) hasDue() bool {
	return r.Kind != kindNote && !r.Due.IsZero()
}

func (r record) label() string {
	return r.Kind + " #" + strconv.FormatInt(r.ID, 10)
}

func dueStamp(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04") + "Z"
}

func (r record) doneMark() string {
	if r.Done {
		return "[x] "
	}
	return "[ ] "
}

func (r record) duePart() string {
	if !r.hasDue() {
		return ""
	}
	return "  (due " + dueStamp(r.Due) + ")"
}

func (r record) text() string {
	title := strings.TrimSpace(r.Title)
	switch r.Kind {
	case kindTask:
		return r.doneMark() + title + r.duePart()
	case kindReminder:
		return title + r.duePart()
	default:
		if strings.TrimSpace(r.Body) == "" {
			return title
		}
		return title + "\n\n" + r.Body
	}
}

func (r record) summary() string {
	var parts []string
	if title := strings.TrimSpace(r.Title); title != "" {
		parts = append(parts, "title="+title)
	}
	if r.Kind == kindNote && r.Body != "" {
		parts = append(parts, "body="+strconv.Itoa(len(r.Body))+" chars")
	}
	if r.hasDue() {
		parts = append(parts, "due="+dueStamp(r.Due))
	}
	if r.Done && r.Kind != kindNote {
		parts = append(parts, "done")
	}
	if len(parts) == 0 {
		return "unsaved draft"
	}
	return strings.Join(parts, "  ")
}

func (r record) preview() recordYAML {
	out := recordYAML{
		Kind:  r.Kind,
		ID:    r.ID,
		Title: strings.TrimSpace(r.Title),
		Done:  r.Done && r.Kind != kindNote,
	}
	if r.Kind == kindNote {
		out.Body = r.Body
	}
	if r.hasDue() {
		out.Due = r.Due.UTC().Format(time.RFC3339)
	}
	return out
}

func (r record) check(th theme.Theme, now time.Time) []string {
	ok, warn := true, false
	var details []string
	switch r.Kind {
	case kindReminder:
		switch {
		case r.Due.IsZero():
			ok = false
			details = append(details, "no due set; a reminder without a due never fires")
		case r.Due.Before(now):
			warn = true
			details = append(details, "due "+dueStamp(r.Due)+" ("+timefmt.RelAt(r.Due, now)+"); fires on the next poll")
		default:
			details = append(details, "fires "+dueStamp(r.Due)+" ("+timefmt.RelAt(r.Due, now)+")")
		}
		if r.Done {
			warn = true
			details = append(details, "already done; it will not fire")
		}
	case kindTask:
		if r.Due.IsZero() {
			details = append(details, "no due set; a task does not need one")
		} else {
			details = append(details, "due "+dueStamp(r.Due)+" ("+timefmt.RelAt(r.Due, now)+")")
		}
		if r.Done {
			details = append(details, "already done")
		}
	default:
		if strings.TrimSpace(r.Body) == "" {
			details = append(details, "no body; the title carries the whole note")
		} else {
			details = append(details, "body "+strconv.Itoa(len(r.Body))+" chars")
		}
	}
	if len(details) == 0 {
		details = append(details, "no problems found")
	}
	lines := []string{recordFindingLine(th, ok && !warn, warn, r.checkName(ok, warn))}
	for _, d := range details {
		lines = append(lines, "    "+th.Dim.Render(d))
	}
	return lines
}

func (r record) checkName(ok, warn bool) string {
	kind := r.Kind
	if kind == "" {
		kind = "record"
	}
	switch {
	case !ok:
		return kind + " will not work"
	case warn:
		return kind + " needs a look"
	default:
		return kind + " looks good"
	}
}

func recordFindingLine(th theme.Theme, ok, warn bool, name string) string {
	var mark string
	switch {
	case ok:
		mark = th.Can.Render(glyph.Check())
	case warn:
		mark = th.Dim.Render(glyph.Warn())
	default:
		mark = th.Cant.Render(glyph.Cross())
	}
	return mark + " " + th.Val.Render(name)
}

func parseDueAt(s string, now time.Time) (time.Time, error) {
	due, err := timefmt.ParseWhenAt(s, now)
	switch {
	case errors.Is(err, timefmt.ErrEmptyTime):
		return now.UTC().Add(time.Hour), nil
	case err != nil:
		return time.Time{}, fmt.Errorf("due: %w", err)
	}
	return due, nil
}
