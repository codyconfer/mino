// Package run builds and runs directives without a cobra command or a TTY, so
// both the CLI and the serve HTTP API can trigger the same work.
package run

import (
	"context"
	"maps"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/app/flight"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/filter"
	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/signals"
	"github.com/codyconfer/mino/internal/signals/build"
)

// errSignal stands in for a query that failed to build, so a flight still
// reports it as a failed section instead of dropping it.
type errSignal struct {
	name string
	err  error
}

func (e errSignal) Name() string { return e.name }

func (e errSignal) Fetch(context.Context) ([]signals.Section, error) { return nil, e.err }

// Signal builds a signal by name with a's stores.
func Signal(a *app.App, name string, params map[string]string) (signals.Signal, error) {
	return build.Signal(name, params, a.Role(), a.Cfg, a.Tokens, a.Cache)
}

// BuildQuery resolves a saved query by name into a runnable flight query.
func BuildQuery(a *app.App, name string) (flight.Query, error) {
	q, ok := a.Dirs().Queries[name]
	if !ok {
		return flight.Query{}, errs.Newf(errs.KindUsage, "no saved query named %q", name).
			WithHint("run `mino query list` to see saved queries")
	}
	if !q.Runnable() {
		return flight.Query{}, errs.Newf(errs.KindUsage, "%q defines no signal, so there is nothing to run", name).
			WithHint("it is a filter-only document; reference it from a query's `filters:` list")
	}
	return BuildQueryFrom(a, name, q)
}

// BuildQueryFrom resolves an already-loaded query definition into a runnable one.
func BuildQueryFrom(a *app.App, label string, q config.Query) (flight.Query, error) {
	resolved, err := a.Dirs().Resolve(q)
	if err != nil {
		return flight.Query{}, err
	}
	params, err := filter.ExpandParams(q.Params, resolved)
	if err != nil {
		return flight.Query{}, errs.Wrapf(errs.KindConfig, err, "query %q", label)
	}
	src, err := Signal(a, q.Signal, params)
	if err != nil {
		return flight.Query{}, errs.Wrapf(errs.KindSignal, err, "query %q", label)
	}
	compiled, err := filter.CompileAll(resolved)
	if err != nil {
		return flight.Query{}, err
	}
	title := q.Display()
	if title == "" {
		title = label
	}
	return flight.Query{Label: label, Title: title, Src: src, Filters: compiled}, nil
}

// FlightQueries builds every query a flight names, substituting a failing
// placeholder for any that cannot be built.
func FlightQueries(a *app.App, name string, names []string) []flight.Query {
	out := make([]flight.Query, 0, len(names))
	for _, n := range names {
		q, err := BuildQuery(a, n)
		if err != nil {
			log.Debugf("flight %q: %v", name, err)
			out = append(out, flight.Query{Label: n, Title: a.Dirs().Queries[n].Display(), Src: errSignal{name: n, err: err}})
			continue
		}
		out = append(out, q)
	}
	return out
}

// Flight runs a named flight's queries and returns their sections. The error is
// non-nil only when every query failed; partial results still come back.
func Flight(ctx context.Context, a *app.App, name string) ([]signals.Section, error) {
	fl, ok := a.Dirs().Flights[name]
	if !ok {
		return nil, errs.Newf(errs.KindUsage, "no flight named %q", name).
			WithHint("run `mino fly` with no argument to list available flights")
	}
	if !a.Access().FlightVisible(name) {
		return nil, a.NotInRoleError("flight", name)
	}
	if len(fl.Queries) == 0 {
		return nil, nil
	}
	queries := FlightQueries(a, name, fl.Queries)

	flightID := a.Audit.StartFlightContext(ctx, name, a.Role())
	defer a.Audit.FinishFlight(flightID)

	sections := flight.FetchQueries(ctx, a.Audit, a.Role(), a.SourceTimeout(), queries, flightID)
	return sections, flight.Failure(sections)
}

// Query runs one saved query and returns its sections. The error is non-nil
// only when the query failed outright; partial results still come back.
func Query(ctx context.Context, a *app.App, name string) ([]signals.Section, error) {
	if _, ok := a.Dirs().Queries[name]; ok && !a.Access().QueryVisible(name) {
		return nil, a.NotInRoleError("query", name)
	}
	q, err := BuildQuery(a, name)
	if err != nil {
		return nil, err
	}
	sections := flight.FetchQuery(ctx, a.Audit, a.Role(), a.SourceTimeout(), q, 0)
	return sections, flight.Failure(sections)
}

// Action runs a registered plugin action, seeding the params every action gets.
func Action(ctx context.Context, a *app.App, signal, name string, params map[string]string) error {
	p := map[string]string{"home": a.Cfg.Home, "role": a.Role()}
	maps.Copy(p, params)
	return build.Action(ctx, signal, name, p)
}
