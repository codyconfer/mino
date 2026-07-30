package serve

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/codyconfer/munin/internal/filter"
	"github.com/codyconfer/munin/internal/log"
	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/signals"
	"github.com/codyconfer/munin/internal/signals/active"
	"github.com/codyconfer/munin/internal/signals/build"
)

func (s *Server) scheduledEvents(ctx context.Context, flight string, names []string, state *active.State) <-chan signals.Event {
	jobs, watching := s.scheduledJobs(flight, names, state)
	if len(jobs) == 0 {
		return nil
	}
	out := make(chan signals.Event, serveBuffer)
	go func() {
		defer close(out)
		err := plugin.RunScheduled(ctx, jobs, func(name string, sections []signals.Section) error {
			for _, sec := range sections {
				if len(sec.Items) == 0 && sec.Err == nil {
					continue
				}
				ev := signals.Event{Source: name, Section: sec, At: time.Now()}
				select {
				case out <- ev:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		})
		if err != nil && ctx.Err() == nil {
			log.Warnf("serve: scheduled: %v", err)
		}
	}()
	fmt.Fprintf(os.Stderr, "watching %-10s %s\n", strings.Join(watching, ","), "(scheduled)")
	return out
}

func (s *Server) scheduledJobs(flight string, names []string, state *active.State) ([]plugin.Scheduled, []string) {
	var jobs []plugin.Scheduled
	var watching []string
	seen := map[string]bool{}
	for _, name := range names {
		q, ok := s.Directives.Queries[name]
		if !ok {
			log.Debugf("serve: unknown query %q in flight %q", name, flight)
			continue
		}
		if !q.Runnable() || seen[q.Signal] {
			continue
		}
		seen[q.Signal] = true
		if !plugin.HasCapability(q.Signal, plugin.CapScheduled) {
			continue
		}
		resolved, err := s.Directives.Resolve(q)
		if err != nil {
			log.Warnf("serve: query %q: %v (skipping)", name, err)
			continue
		}
		params, err := filter.ExpandParams(q.Params, resolved)
		if err != nil {
			log.Warnf("serve: query %q: %v (skipping)", name, err)
			continue
		}
		job, err := build.ScheduledJob(q.Signal, params, s.Cfg, s.Tokens, state)
		if err != nil {
			if errors.Is(err, build.ErrNoScheduled) {
				log.Debugf("serve: query %q signal %q has no scheduled support (skipping)", name, q.Signal)
			} else {
				log.Warnf("serve: query %q: %v (skipping)", name, err)
			}
			continue
		}
		jobs = append(jobs, job)
		watching = append(watching, q.Signal)
	}
	return jobs, watching
}
