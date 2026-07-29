package flight

import (
	"context"
	"io"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/codyconfer/munin/internal/audit"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/filter"
	"github.com/codyconfer/munin/internal/render"
	"github.com/codyconfer/munin/internal/signals"
)

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

func FetchQuery(ctx context.Context, au *audit.Store, role string, timeout time.Duration, q Query, parentID int64) []signals.Section {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	started := time.Now()
	sections, err := q.Src.Fetch(ctx)
	if err != nil {
		sections = []signals.Section{{Signal: q.Src.Name(), Title: q.Src.Name(), Err: err}}
	} else {
		for i := range sections {
			sections[i].Items = filter.ApplyAll(q.Filters, sections[i].Items)
		}
	}
	label := q.Label
	if label == "" {
		label = q.Src.Name()
	}
	au.RecordQuery(parentID, label, role, started, time.Now(), sections)
	return sections
}

func FetchGroups(ctx context.Context, au *audit.Store, role string, timeout time.Duration, queries []Query, parentID int64) []Group {
	results := make([]Group, len(queries))
	g, ctx := errgroup.WithContext(ctx)
	for i, q := range queries {
		g.Go(func() error {
			label := q.Label
			if label == "" {
				label = q.Src.Name()
			}
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
