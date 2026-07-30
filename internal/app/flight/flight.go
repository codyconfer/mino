package flight

import (
	"context"
	"errors"
	"io"
	"os"
	"runtime/debug"
	"strconv"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/codyconfer/munin/internal/audit"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/filter"
	"github.com/codyconfer/munin/internal/log"
	"github.com/codyconfer/munin/internal/render"
	"github.com/codyconfer/munin/internal/signals"
)

const (
	DefaultFetchLimit = 8
	FetchLimitEnv     = "MUNIN_FETCH_CONCURRENCY"

	unknownSignal = "unknown"
)

var ErrAllQueriesFailed = errors.New("every query failed")

func FetchLimit() int {
	if v := os.Getenv(FetchLimitEnv); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
		log.Debugf("ignoring %s=%q: want a positive integer", FetchLimitEnv, v)
	}
	return DefaultFetchLimit
}

type Query struct {
	Label   string
	Title   string
	Src     signals.Signal
	Filters []filter.Compiled
}

func (q Query) Display() string {
	if q.Title != "" {
		return q.Title
	}
	return q.Label
}

type Group struct {
	Query, Title string
	Sections     []signals.Section
}

func Emit(w io.Writer, format, root string, sections []signals.Section) error {
	r, err := render.New(render.Format(format), root)
	if err != nil {
		return errs.Wrapf(errs.KindConfig, err, "invalid output format %q", format).
			WithHint("set output to terminal or json")
	}
	return r.Render(w, sections)
}

func signalName(q Query) (name string) {
	name = q.Label
	if name == "" {
		name = unknownSignal
	}
	defer func() {
		if r := recover(); r != nil {
			log.Debugf("signal name panicked for query %q: %v", q.Label, r)
		}
	}()
	if q.Src != nil {
		if n := q.Src.Name(); n != "" {
			name = n
		}
	}
	return name
}

func panicErr(name string, r any) error {
	return errs.Newf(errs.KindInternal, "signal %s panicked: %v", name, r).
		WithHint("this is a bug in the %s signal; run with MUNIN_LOG_LEVEL=debug for the stack trace", name)
}

func errSection(name string, err error) []signals.Section {
	return []signals.Section{{Signal: name, Title: name, Err: err}}
}

func fetchSections(ctx context.Context, name string, q Query) (sections []signals.Section, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Debugf("panic in signal %s: %v\n%s", name, r, debug.Stack())
			sections, err = nil, panicErr(name, r)
		}
	}()
	if q.Src == nil {
		return nil, errs.Newf(errs.KindConfig, "query %s has no signal source", name).
			WithHint("check the query definition for %s", name)
	}
	return q.Src.Fetch(ctx)
}

func FetchQuery(ctx context.Context, au *audit.Store, role string, timeout time.Duration, q Query, parentID int64) []signals.Section {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	name := signalName(q)
	started := time.Now()
	sections, err := fetchSections(ctx, name, q)
	if err != nil {
		sections = errSection(name, err)
	} else {
		for i := range sections {
			sections[i].Items = filter.ApplyAll(q.Filters, sections[i].Items)
		}
	}
	label := q.Label
	if label == "" {
		label = name
	}
	au.RecordQuery(parentID, label, role, started, time.Now(), sections)
	return sections
}

func FetchGroups(ctx context.Context, au *audit.Store, role string, timeout time.Duration, queries []Query, parentID int64) []Group {
	results := make([]Group, len(queries))
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(FetchLimit())
	for i, q := range queries {
		g.Go(func() error {
			name := signalName(q)
			label := q.Label
			if label == "" {
				label = name
			}
			defer func() {
				if r := recover(); r != nil {
					log.Debugf("panic while running query %s: %v\n%s", label, r, debug.Stack())
					results[i] = Group{Query: label, Title: q.Display(), Sections: errSection(name, panicErr(name, r))}
				}
			}()
			results[i] = Group{
				Query:    label,
				Title:    q.Display(),
				Sections: FetchQuery(ctx, au, role, timeout, q, parentID),
			}
			return nil
		})
	}
	_ = g.Wait()
	return results
}

func Flatten(groups []Group) []signals.Section {
	var all []signals.Section
	for _, g := range groups {
		all = append(all, g.Sections...)
	}
	return all
}

func FetchQueries(ctx context.Context, au *audit.Store, role string, timeout time.Duration, queries []Query, parentID int64) []signals.Section {
	return Flatten(FetchGroups(ctx, au, role, timeout, queries, parentID))
}

type Outcome struct {
	Sections int
	Failed   int
	Items    int
}

func Tally(sections []signals.Section) Outcome {
	var o Outcome
	o.Sections = len(sections)
	for _, s := range sections {
		if s.Err != nil {
			o.Failed++
		}
		o.Items += len(s.Items)
	}
	return o
}

func (o Outcome) TotalFailure() bool {
	return o.Sections > 0 && o.Failed == o.Sections && o.Items == 0
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func Failure(sections []signals.Section) error {
	o := Tally(sections)
	if !o.TotalFailure() {
		return nil
	}
	return errs.Wrapf(errs.KindSignal, ErrAllQueriesFailed, "no results from %d source%s", o.Sections, plural(o.Sections)).
		WithHint("see the per-source errors above")
}
