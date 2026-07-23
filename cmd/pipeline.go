package cmd

import (
	"context"
	"os"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/filter"
	"github.com/codyconfer/munin/internal/log"
	"github.com/codyconfer/munin/internal/render"
	"github.com/codyconfer/munin/internal/signals"
)

const defaultSignalTimeout = 30 * time.Second

func sourceTimeout() time.Duration {
	if shared.cfg != nil && shared.cfg.Timeout != "" {
		if d, err := time.ParseDuration(shared.cfg.Timeout); err == nil && d > 0 {
			return d
		}
	}
	return defaultSignalTimeout
}

type job struct {
	label   string
	src     signals.Signal
	filters []filter.Compiled
}

func emit(sections []signals.Section) error {
	r, err := render.New(render.Format(shared.cfg.Output))
	if err != nil {
		return errs.Wrapf(errs.KindConfig, err, "invalid output format %q", shared.cfg.Output).
			WithHint("set output to terminal or json")
	}
	return r.Render(os.Stdout, sections)
}

func fetchOne(ctx context.Context, j job, parentID int64) []signals.Section {
	ctx, cancel := context.WithTimeout(ctx, sourceTimeout())
	defer cancel()

	started := time.Now()
	sections, err := j.src.Fetch(ctx)
	if err != nil {
		sections = []signals.Section{{Signal: j.src.Name(), Title: j.src.Name(), Err: err}}
	} else {
		for i := range sections {
			sections[i].Items = filter.ApplyAll(j.filters, sections[i].Items)
		}
	}
	label := j.label
	if label == "" {
		label = j.src.Name()
	}
	shared.audit.RecordQuery(parentID, label, shared.cfg.Role, started, time.Now(), sections)
	return sections
}

func fetchJobs(ctx context.Context, jobs []job, parentID int64) []signals.Section {
	results := make([][]signals.Section, len(jobs))
	g, ctx := errgroup.WithContext(ctx)
	for i, j := range jobs {
		g.Go(func() error {
			results[i] = fetchOne(ctx, j, parentID)
			return nil
		})
	}
	_ = g.Wait()

	var all []signals.Section
	for _, r := range results {
		all = append(all, r...)
	}
	return all
}

func verbosef(format string, args ...any) {
	log.Debugf(format, args...)
}
