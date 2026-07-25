package daemon

import (
	"context"
	"fmt"
	"os"
	"time"

	sysdaemon "github.com/codyconfer/sisyphus/daemon"

	"github.com/codyconfer/munin/internal/log"
	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/plugin/ntr"
	"github.com/codyconfer/munin/internal/signals"
	"github.com/codyconfer/munin/internal/signals/active"
)

func (s *Server) scheduledEvents(ctx context.Context, _ string, names []string, state *active.State) <-chan signals.Event {
	var jobs []plugin.Scheduled
	seen := map[string]bool{}
	role := s.Cfg.Role
	if role == "" {
		role = "default"
	}
	var kvStore sysdaemon.KV
	if state != nil {
		kvStore = state.KV()
	}
	for _, name := range names {
		q, ok := s.Directives.Queries[name]
		if !ok || seen[q.Signal] {
			continue
		}
		seen[q.Signal] = true
		switch q.Signal {
		case ntr.SignalName:
			if !plugin.SignalEnabled(ntr.SignalName) {
				continue
			}
			jobs = append(jobs, ntr.ReminderJob{Home: s.Cfg.Home, Role: role, KV: kvStore})
		}
	}
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
	fmt.Fprintf(os.Stderr, "watching %-10s %s\n", "ntr", "(scheduled reminders)")
	return out
}
