package gcal

import (
	"context"
	"errors"
	"net/http"
	"time"

	"google.golang.org/api/googleapi"

	calendar "google.golang.org/api/calendar/v3"

	"github.com/codyconfer/munin/internal/auth"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/signals"
	"github.com/codyconfer/munin/internal/signals/active"
)

func NewActive(calendarID string, ga auth.GoogleAuth, interval time.Duration, state *active.State) signals.ActiveSignal {
	if calendarID == "" {
		calendarID = "primary"
	}
	return &gcalActive{calendarID: calendarID, auth: ga, interval: interval, state: state}
}

type gcalActive struct {
	calendarID string
	auth       auth.GoogleAuth
	interval   time.Duration
	state      *active.State
}

func (h *gcalActive) Name() string { return "calendar" }

func (h *gcalActive) LatencyFloor() time.Duration { return h.interval }

func (h *gcalActive) Stream(ctx context.Context) (<-chan signals.Event, error) {
	var svc *calendar.Service
	cursor := h.state.Cursor("calendar", h.calendarID+":sync_token")
	syncToken := cursor.Load(ctx)
	setSync := func(v string) {
		if v == syncToken {
			return
		}
		syncToken = v
		_ = cursor.Save(ctx, v)
	}

	ensure := func(ctx context.Context) error {
		if svc != nil {
			return nil
		}
		s, err := auth.GoogleService(ctx, h.auth, "calendar", calendar.CalendarReadonlyScope, calendar.NewService)
		if err != nil {
			return err
		}
		svc = s
		return nil
	}

	baseline := func(ctx context.Context) error {
		token := ""
		pageToken := ""
		for {
			call := svc.Events.List(h.calendarID).
				SingleEvents(true).
				ShowDeleted(true).
				Context(ctx)
			if pageToken != "" {
				call = call.PageToken(pageToken)
			}
			res, err := call.Do()
			if err != nil {
				return errs.Wrapf(errs.KindSignal, err, "calendar: baseline sync for %q", h.calendarID)
			}
			if res.NextSyncToken != "" {
				token = res.NextSyncToken
			}
			if res.NextPageToken == "" {
				break
			}
			pageToken = res.NextPageToken
		}
		setSync(token)
		return nil
	}

	step := func(ctx context.Context) ([]signals.Item, error) {
		if err := ensure(ctx); err != nil {
			return nil, err
		}
		if syncToken == "" {
			return nil, baseline(ctx)
		}

		var items []signals.Item
		pageToken := ""
		for {
			call := svc.Events.List(h.calendarID).
				SyncToken(syncToken).
				ShowDeleted(true).
				Context(ctx)
			if pageToken != "" {
				call = call.PageToken(pageToken)
			}
			res, err := call.Do()
			if err != nil {
				var gerr *googleapi.Error
				if errors.As(err, &gerr) && gerr.Code == http.StatusGone {
					setSync("")
					return nil, baseline(ctx)
				}
				return nil, errs.Wrapf(errs.KindSignal, err, "calendar: incremental sync for %q", h.calendarID)
			}
			for _, ev := range res.Items {
				if ev.Status == "cancelled" {
					continue
				}
				items = append(items, eventToItem(ev))
			}
			if res.NextSyncToken != "" {
				setSync(res.NextSyncToken)
			}
			if res.NextPageToken == "" {
				break
			}
			pageToken = res.NextPageToken
		}
		return items, nil
	}

	return active.Poll(ctx, "calendar", h.interval, step), nil
}
