package gcal

import (
	"context"
	"time"

	calendar "google.golang.org/api/calendar/v3"

	"github.com/codyconfer/munin/internal/auth"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/signals"
)

const defaultWindow = 24 * time.Hour

type gcalSignal struct {
	calendarID string
	window     time.Duration
	max        int
	auth       auth.GoogleAuth
}

func New(calendarID string, window time.Duration, max int, ga auth.GoogleAuth) signals.Signal {
	return &gcalSignal{calendarID: calendarID, window: window, max: max, auth: ga}
}

func (s *gcalSignal) Name() string { return "calendar" }

func (s *gcalSignal) Fetch(ctx context.Context) ([]signals.Section, error) {
	opt, err := auth.GoogleClientOption(ctx, s.auth, calendar.CalendarReadonlyScope)
	if err != nil {
		return nil, err
	}
	svc, err := calendar.NewService(ctx, opt)
	if err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "calendar: creating service")
	}

	calendarID := s.calendarID
	if calendarID == "" {
		calendarID = "primary"
	}
	window := s.window
	if window <= 0 {
		window = defaultWindow
	}
	max := s.max
	if max <= 0 {
		max = 50
	}

	now := time.Now()
	res, err := svc.Events.List(calendarID).
		ShowDeleted(false).
		SingleEvents(true).
		OrderBy("startTime").
		TimeMin(now.Format(time.RFC3339)).
		TimeMax(now.Add(window).Format(time.RFC3339)).
		MaxResults(int64(max)).
		Context(ctx).
		Do()
	if err != nil {
		return nil, errs.Wrapf(errs.KindSignal, err, "calendar: listing events for %q", calendarID)
	}

	items := make([]signals.Item, 0, len(res.Items))
	for _, ev := range res.Items {
		items = append(items, eventToItem(ev))
	}

	return []signals.Section{{
		Signal: "calendar",
		Title:  "Calendar",
		Items:  items,
	}}, nil
}

func eventToItem(ev *calendar.Event) signals.Item {
	item := signals.Item{
		Kind:     "event",
		Title:    ev.Summary,
		Subtitle: ev.Location,
		Body:     ev.Description,
		URL:      ev.HtmlLink,
	}

	if ev.Organizer != nil && ev.Organizer.Email != "" {
		item.Meta = map[string]string{"organizer": ev.Organizer.Email}
	}

	if ev.Start != nil {
		if ev.Start.DateTime != "" {
			if t, err := time.Parse(time.RFC3339, ev.Start.DateTime); err == nil {
				item.Timestamp = t
			}
		} else if ev.Start.Date != "" {
			if t, err := time.Parse("2006-01-02", ev.Start.Date); err == nil {
				item.Timestamp = t
			}
		}
	}

	return item
}
