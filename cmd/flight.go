package cmd

import (
	"context"
	"io"
	"strconv"
	"time"

	"github.com/codyconfer/munin/internal/app/flight"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/format"
	"github.com/codyconfer/munin/internal/log"
	"github.com/codyconfer/munin/internal/render"
	"github.com/codyconfer/munin/internal/signals"
)

func sourceTimeout() time.Duration { return shared.SourceTimeout() }

type query = flight.Query

func emit(w io.Writer, root string, sections []signals.Section) error {
	return flight.Emit(w, shared.Cfg.Output, root, sections)
}

func fetchQuery(ctx context.Context, q query, parentID int64) []signals.Section {
	return flight.FetchQuery(ctx, shared.Audit, shared.Cfg.Role, sourceTimeout(), q, parentID)
}

func fetchQueries(ctx context.Context, queries []query, parentID int64) []signals.Section {
	return flight.FetchQueries(ctx, shared.Audit, shared.Cfg.Role, sourceTimeout(), queries, parentID)
}

func fetchGroups(ctx context.Context, queries []query, parentID int64) []flight.Group {
	return flight.FetchGroups(ctx, shared.Audit, shared.Cfg.Role, sourceTimeout(), queries, parentID)
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
			return emit(w, root, fetchQueries(ctx, queries, parentID))
		}
		tasks := make([]deck.Task, len(queries))
		for i, q := range queries {
			tasks[i] = deck.Task{
				Label: q.Display(),
				Run:   func(ctx context.Context) []signals.Section { return fetchQuery(ctx, q, parentID) },
			}
		}
		return deck.RunFlight(ctx, tasks)
	}
	groups := fetchGroupsLoading(ctx, status, o.formatter, queries, parentID)
	return deliverGroups(w, status, o, root, groups)
}

func fetchGroupsLoading(ctx context.Context, status io.Writer, fd config.FormatterDef, queries []query, parentID int64) []flightGroup {
	if !interactiveTTY() {
		return fetchGroups(ctx, queries, parentID)
	}
	sp := render.StartLoading(render.LoadingOptions{
		Writer:  status,
		Message: "running " + strconv.Itoa(len(queries)) + " quer" + plural(len(queries), "y", "ies") + " for " + fd.Display(),
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
		Role:      shared.Cfg.Role,
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
