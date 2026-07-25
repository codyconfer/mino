package ntr

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/codyconfer/sisyphus/daemon"
	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/signals"
)

const (
	PluginID   = "munin.ntr"
	SignalName = "ntr"
	GlyphID    = "munin.ntr"
)

func init() {
	plugin.Register(plugin.Descriptor{
		ID:     PluginID,
		Kind:   plugin.KindSignal,
		Signal: SignalName,
		Capabilities: []plugin.Capability{
			plugin.CapQuery, plugin.CapAction, plugin.CapScheduled,
		},
	})
	plugin.RegisterBuilders(SignalName, plugin.Builders{
		Query: func(bc plugin.BuildContext) (plugin.Query, error) {
			role := bc.Role()
			if role == "" {
				role = "default"
			}
			return Signal{Home: bc.Home(), Role: role}, nil
		},
	})
	glyph.Register(GlyphID, glyph.Variants{Nerd: "", Uni: "✎", ASCII: "nt"})
	plugin.RegisterStatusContribution(PluginID, StatusContribution)
}

// Signal lists notes/tasks for the active role (Query capability).
type Signal struct {
	Home string
	Role string
}

func (s Signal) Name() string { return SignalName }

func (s Signal) Fetch(ctx context.Context) ([]signals.Section, error) {
	st, err := Open(ctx, s.Home, s.Role)
	if err != nil {
		return nil, err
	}
	defer st.Close()
	notes, err := st.ListNotes(ctx)
	if err != nil {
		return nil, err
	}
	tasks, err := st.ListTasks(ctx, false)
	if err != nil {
		return nil, err
	}
	var items []signals.Item
	for _, n := range notes {
		items = append(items, signals.Item{
			Kind:  "note",
			Title: n.Title,
			Body:  n.Body,
			Meta:  map[string]string{"id": fmt.Sprint(n.ID), "type": "note"},
		})
	}
	for _, t := range tasks {
		kind := "task"
		if t.Done {
			kind = "completed"
		}
		items = append(items, signals.Item{
			Kind:  kind,
			Title: t.Title,
			Meta:  map[string]string{"id": fmt.Sprint(t.ID), "type": "task"},
		})
	}
	return []signals.Section{{Signal: SignalName, Title: "notes", Items: items}}, nil
}

// ReminderJob implements Scheduled for due reminders.
// Fetch is delivery-only; Ack marks done + watermark after notify succeeds.
type ReminderJob struct {
	Home string
	Role string
	KV   daemon.KV
	Now  func() time.Time
}

func (r ReminderJob) Name() string { return SignalName }

func (r ReminderJob) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r ReminderJob) Next(ctx context.Context, now time.Time) (due time.Time, ready bool, err error) {
	st, err := Open(ctx, r.Home, r.Role)
	if err != nil {
		return time.Time{}, false, err
	}
	defer st.Close()
	dueList, err := st.DueReminders(ctx, now)
	if err != nil {
		return time.Time{}, false, err
	}
	if len(dueList) > 0 {
		return time.Time{}, true, nil
	}
	return now.Add(time.Minute), false, nil
}

// Fetch loads due reminders for delivery. It does not mark them done or stamp
// the watermark — call Ack after the notify sink accepts the event.
func (r ReminderJob) Fetch(ctx context.Context) ([]signals.Section, error) {
	st, err := Open(ctx, r.Home, r.Role)
	if err != nil {
		return nil, err
	}
	defer st.Close()
	now := r.now()
	due, err := st.DueReminders(ctx, now)
	if err != nil {
		return nil, err
	}
	var items []signals.Item
	for _, rem := range due {
		items = append(items, signals.Item{
			Kind:      "alert",
			Title:     rem.Title,
			Timestamp: rem.Due,
			Meta:      map[string]string{"id": fmt.Sprint(rem.ID), "type": "reminder"},
		})
	}
	return []signals.Section{{Signal: SignalName, Title: "reminders", Items: items}}, nil
}

// Ack marks delivered reminder IDs done and advances the catch-up watermark.
// Safe to call with empty sections (no-op).
func (r ReminderJob) Ack(ctx context.Context, sections []signals.Section) error {
	var ids []int64
	for _, sec := range sections {
		for _, it := range sec.Items {
			if it.Meta["type"] != "reminder" {
				continue
			}
			id, err := strconv.ParseInt(it.Meta["id"], 10, 64)
			if err != nil || id == 0 {
				continue
			}
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	st, err := Open(ctx, r.Home, r.Role)
	if err != nil {
		return err
	}
	defer st.Close()
	for _, id := range ids {
		if err := st.MarkReminderDone(ctx, id); err != nil {
			return err
		}
	}
	if r.KV != nil {
		if err := daemon.NewWatermark(r.KV, "ntr", "reminders:"+r.Role).Save(ctx, r.now()); err != nil {
			return err
		}
	}
	return nil
}

// StatusContribution reports due-today reminder count.
func StatusContribution(home, role string) glyph.StatusContribution {
	return glyph.StatusContribution{
		BrandGlyph: glyph.ResolveID(GlyphID),
		Info:       func() string { return "notes" },
		Status: func() (string, glyph.Severity) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			st, err := Open(ctx, home, role)
			if err != nil {
				return glyph.StatusMuted(), glyph.SeverityNeutral
			}
			defer st.Close()
			n, err := st.DueTodayCount(ctx, time.Now())
			if err != nil || n == 0 {
				return glyph.StatusOK(), glyph.SeverityPositive
			}
			return fmt.Sprintf("%s%d", glyph.Clock(), n), glyph.SeverityWarning
		},
	}
}
