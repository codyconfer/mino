package gtasks

import (
	"context"
	"strings"
	"time"

	tasksapi "google.golang.org/api/tasks/v1"

	"github.com/codyconfer/munin/internal/auth"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/signals"
)

const maxResults = 100

type gtasksSignal struct {
	auth          auth.GoogleAuth
	lists         []string
	showCompleted bool
	max           int
}

func New(ga auth.GoogleAuth, lists []string, showCompleted bool, max int) signals.Signal {
	return &gtasksSignal{auth: ga, lists: lists, showCompleted: showCompleted, max: max}
}

func (s *gtasksSignal) Name() string { return "tasks" }

func (s *gtasksSignal) Fetch(ctx context.Context) ([]signals.Section, error) {
	svc, err := newService(ctx, s.auth)
	if err != nil {
		return nil, err
	}
	lists, err := svc.Tasklists.List().MaxResults(maxResults).Context(ctx).Do()
	if err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "tasks: listing task lists")
	}

	perList := s.max
	if perList <= 0 {
		perList = maxResults
	}
	var sections []signals.Section
	for _, tl := range lists.Items {
		if !s.wantList(tl) {
			continue
		}
		res, err := svc.Tasks.List(tl.Id).
			ShowCompleted(s.showCompleted).
			MaxResults(int64(perList)).
			Context(ctx).Do()
		if err != nil {
			return nil, errs.Wrapf(errs.KindSignal, err, "tasks: listing tasks in %q", tl.Title)
		}
		sec := signals.Section{Signal: "tasks", Title: "Tasks: " + tl.Title}
		for _, t := range res.Items {
			sec.Items = append(sec.Items, taskToItem(t, tl.Title))
		}
		sections = append(sections, sec)
	}
	return sections, nil
}

func (s *gtasksSignal) wantList(tl *tasksapi.TaskList) bool {
	if len(s.lists) == 0 {
		return true
	}
	for _, want := range s.lists {
		if want == tl.Id || strings.EqualFold(want, tl.Title) {
			return true
		}
	}
	return false
}

func CreateTask(ctx context.Context, ga auth.GoogleAuth, listRef, title, notes, due string) (signals.Item, error) {
	svc, err := newService(ctx, ga)
	if err != nil {
		return signals.Item{}, err
	}
	id, listTitle, err := resolveList(ctx, svc, listRef)
	if err != nil {
		return signals.Item{}, err
	}
	t := &tasksapi.Task{Title: title, Notes: notes}
	if due != "" {
		d, err := normalizeDue(due)
		if err != nil {
			return signals.Item{}, err
		}
		t.Due = d
	}
	created, err := svc.Tasks.Insert(id, t).Context(ctx).Do()
	if err != nil {
		return signals.Item{}, errs.Wrapf(errs.KindSignal, err, "tasks: creating task in %q", listTitle)
	}
	return taskToItem(created, listTitle), nil
}

func resolveList(ctx context.Context, svc *tasksapi.Service, ref string) (id, title string, err error) {
	lists, err := svc.Tasklists.List().MaxResults(maxResults).Context(ctx).Do()
	if err != nil {
		return "", "", errs.Wrap(errs.KindSignal, err, "tasks: listing task lists")
	}
	var names []string
	for _, tl := range lists.Items {
		names = append(names, tl.Title)
		if tl.Id == ref || strings.EqualFold(tl.Title, ref) {
			return tl.Id, tl.Title, nil
		}
	}
	return "", "", errs.Newf(errs.KindSignal, "tasks: task list %q not found", ref).
		WithHint("available lists: %s", strings.Join(names, ", "))
}

func newService(ctx context.Context, ga auth.GoogleAuth) (*tasksapi.Service, error) {
	return auth.GoogleService(ctx, ga, "tasks", []string{tasksapi.TasksScope}, tasksapi.NewService)
}

func taskToItem(t *tasksapi.Task, listTitle string) signals.Item {
	var ts time.Time
	switch {
	case t.Due != "":
		ts, _ = time.Parse(time.RFC3339, t.Due)
	case t.Updated != "":
		ts, _ = time.Parse(time.RFC3339, t.Updated)
	}
	meta := map[string]string{}
	if t.Id != "" {
		meta["id"] = t.Id
	}
	if t.Updated != "" {
		meta["updated"] = t.Updated
	}
	if t.Status != "" {
		meta["status"] = t.Status
	}
	return signals.Item{
		Kind:      "task",
		Title:     t.Title,
		Subtitle:  listTitle,
		Body:      t.Notes,
		URL:       t.WebViewLink,
		Timestamp: ts,
		Meta:      meta,
	}
}

func normalizeDue(due string) (string, error) {
	if len(due) == 10 {
		if _, err := time.Parse("2006-01-02", due); err != nil {
			return "", errs.Newf(errs.KindUsage, "invalid due date %q", due).
				WithHint("use YYYY-MM-DD or an RFC3339 timestamp")
		}
		return due + "T00:00:00Z", nil
	}
	if _, err := time.Parse(time.RFC3339, due); err != nil {
		return "", errs.Newf(errs.KindUsage, "invalid due date %q", due).
			WithHint("use YYYY-MM-DD or an RFC3339 timestamp")
	}
	return due, nil
}
