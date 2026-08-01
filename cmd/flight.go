package cmd

import (
	"context"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/codyconfer/mino/internal/app/flight"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/deck"
	"github.com/codyconfer/mino/internal/format"
	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/render"
	"github.com/codyconfer/mino/internal/signals"
)

func sourceTimeout() time.Duration { return shared.SourceTimeout() }

type query = flight.Query

func emit(w io.Writer, root string, sections []signals.Section) error {
	return flight.Emit(w, shared.Cfg.Output, root, sections)
}

func fetchQuery(ctx context.Context, q query, parentID int64) []signals.Section {
	return flight.FetchQuery(ctx, shared.Audit, shared.Role(), sourceTimeout(), q, parentID)
}

func fetchQueries(ctx context.Context, queries []query, parentID int64) []signals.Section {
	return flight.FetchQueries(ctx, shared.Audit, shared.Role(), sourceTimeout(), queries, parentID)
}

func fetchGroups(ctx context.Context, queries []query, parentID int64) []flight.Group {
	return flight.FetchGroups(ctx, shared.Audit, shared.Role(), sourceTimeout(), queries, parentID)
}

type flightGroup = flight.Group

type runOpts struct {
	formatter config.FormatterDef
	active    bool
	copyOut   bool
	out       string
	kind      string
}

func runQueries(ctx context.Context, w io.Writer, root string, queries []query, parentID int64) error {
	return runQueriesWith(ctx, w, io.Discard, root, queries, parentID, runOpts{})
}

func runQueriesWith(ctx context.Context, w, status io.Writer, root string, queries []query, parentID int64, o runOpts) error {
	if !o.active {
		if !interactiveTTY() {
			sections := fetchQueries(ctx, queries, parentID)
			if err := emit(w, root, sections); err != nil {
				return err
			}
			return flight.Failure(sections)
		}
		var seen sectionTally
		tasks := make([]deck.Task, len(queries))
		for i, q := range queries {
			tasks[i] = deck.Task{
				Label: q.Display(),
				Run: func(ctx context.Context) []signals.Section {
					return seen.record(fetchQuery(ctx, q, parentID))
				},
			}
		}
		if err := deck.RunFlight(ctx, tasks); err != nil {
			return err
		}
		return flight.Failure(seen.sections())
	}
	groups := fetchGroupsLoading(ctx, status, o.formatter, queries, parentID)
	if err := deliverGroups(w, status, o, root, groups); err != nil {
		return err
	}
	return flight.Failure(flight.Flatten(groups))
}

type sectionTally struct {
	mu  sync.Mutex
	all []signals.Section
}

func (t *sectionTally) record(sections []signals.Section) []signals.Section {
	t.mu.Lock()
	t.all = append(t.all, sections...)
	t.mu.Unlock()
	return sections
}

func (t *sectionTally) sections() []signals.Section {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.all
}

func fetchGroupsLoading(ctx context.Context, status io.Writer, fd config.FormatterDef, queries []query, parentID int64) []flightGroup {
	if !interactiveTTY() {
		return fetchGroups(ctx, queries, parentID)
	}
	sp := render.StartLoading(render.LoadingOptions{
		Writer:  status,
		Message: "running " + strconv.Itoa(len(queries)) + " quer" + plural(len(queries), "y", "ies") + " for " + fd.Display(),
		UI:      Scope(),
	})
	groups := fetchGroups(ctx, queries, parentID)
	sp.Stop()
	return groups
}

func renderReport(fd config.FormatterDef, root, kind string, groups []flightGroup) (string, error) {
	if kind == "" {
		kind = "flight"
	}
	in := format.Input{
		Formatter: fd.Name,
		Name:      root,
		Kind:      kind,
		Role:      shared.Role(),
		Groups:    make([]format.InputGroup, 0, len(groups)),
	}
	for _, g := range groups {
		in.Groups = append(in.Groups, format.InputGroup{Query: g.Query, Title: g.Title, Sections: g.Sections})
	}
	return format.Render(fd.Name, fd.Template, format.Build(in))
}

func deliverGroups(w, status io.Writer, o runOpts, root string, groups []flightGroup) error {
	text, err := renderReport(o.formatter, root, o.kind, groups)
	if err != nil {
		return err
	}
	return format.Deliver(w, status, text, copyFunc(o), o.out)
}

func verbosef(msg string, args ...any) {
	log.Debugf(msg, args...)
}
