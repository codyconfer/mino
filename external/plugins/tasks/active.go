package tasks

import (
	"context"
	"strings"
	"time"

	tasksapi "google.golang.org/api/tasks/v1"

	"github.com/codyconfer/munin/external/plugins/internal/errx"
	"github.com/codyconfer/munin/external/plugins/internal/googleauth"
	"github.com/codyconfer/munin/external/plugins/internal/stream"
	"github.com/codyconfer/munin/plugin"
)

type activeTasks struct {
	auth          googleauth.Auth
	lists         []string
	showCompleted bool
	interval      time.Duration
	state         *stream.State
}

func NewActive(ga googleauth.Auth, lists []string, showCompleted bool, interval time.Duration, state *stream.State) plugin.Stream {
	return &activeTasks{auth: ga, lists: lists, showCompleted: showCompleted, interval: interval, state: state}
}

func (h *activeTasks) Name() string { return "tasks" }

func (h *activeTasks) LatencyFloor() time.Duration { return h.interval }

func (h *activeTasks) Stream(ctx context.Context) (<-chan plugin.Event, error) {
	var svc *tasksapi.Service
	seen := h.state.Seen("tasks")

	step := func(ctx context.Context) ([]plugin.Item, error) {
		if svc == nil {
			s, err := newService(ctx, h.auth)
			if err != nil {
				return nil, err
			}
			svc = s
		}
		lists, err := svc.Tasklists.List().MaxResults(maxResults).Context(ctx).Do()
		if err != nil {
			return nil, errx.Wrap(err, "tasks: listing task lists")
		}
		var items []plugin.Item
		for _, tl := range lists.Items {
			if !h.wantList(tl) {
				continue
			}
			res, err := svc.Tasks.List(tl.Id).
				ShowCompleted(h.showCompleted).
				ShowHidden(true).
				MaxResults(maxResults).
				Context(ctx).Do()
			if err != nil {
				return nil, errx.Wrapf(err, "tasks: listing tasks in %q", tl.Title)
			}
			for _, t := range res.Items {
				items = append(items, taskToItem(t, tl.Title))
			}
		}
		return seen.Fresh(ctx, items, taskKey), nil
	}

	return stream.Poll(ctx, "tasks", h.interval, step), nil
}

func (h *activeTasks) wantList(tl *tasksapi.TaskList) bool {
	if len(h.lists) == 0 {
		return true
	}
	for _, want := range h.lists {
		if want == tl.Id || strings.EqualFold(want, tl.Title) {
			return true
		}
	}
	return false
}

func taskKey(it plugin.Item) string {
	if id := it.Meta["id"]; id != "" {
		return id + "|" + it.Meta["updated"]
	}
	return it.Title + "|" + it.Timestamp.String() + "|" + it.Body
}
