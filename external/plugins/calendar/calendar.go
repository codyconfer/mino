package calendar

import (
	"context"
	"time"

	calapi "google.golang.org/api/calendar/v3"

	"github.com/codyconfer/mino/external/plugins/internal/errx"
	"github.com/codyconfer/mino/external/plugins/internal/googleauth"
	"github.com/codyconfer/mino/plugin"
)

const defaultWindow = 24 * time.Hour

type gcalSignal struct {
	calendarID string
	window     time.Duration
	max        int
	auth       googleauth.Auth
}

func New(calendarID string, window time.Duration, max int, ga googleauth.Auth) plugin.Query {
	return &gcalSignal{calendarID: calendarID, window: window, max: max, auth: ga}
}

func (s *gcalSignal) Name() string { return "calendar" }

func (s *gcalSignal) Fetch(ctx context.Context) ([]plugin.Section, error) {
	svc, err := googleauth.Service(ctx, s.auth, "calendar", []string{calapi.CalendarReadonlyScope}, calapi.NewService)
	if err != nil {
		return nil, err
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
		return nil, errx.Wrapf(err, "calendar: listing events for %q", calendarID)
	}

	items := make([]plugin.Item, 0, len(res.Items))
	for _, ev := range res.Items {
		items = append(items, eventToItem(ev))
	}

	return []plugin.Section{{
		Signal: "calendar",
		Title:  "Calendar",
		Items:  items,
	}}, nil
}

func eventToItem(ev *calapi.Event) plugin.Item {
	item := plugin.Item{
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
