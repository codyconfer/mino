package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/filter"
	"github.com/codyconfer/munin/internal/hot"
	mnotify "github.com/codyconfer/munin/internal/notify"
	"github.com/codyconfer/munin/internal/signals"
	"github.com/codyconfer/munin/internal/signals/demo"
)

var errNoHotSignal = errs.New(errs.KindUsage, "signal has no realtime (hot) implementation")

type hotJob struct {
	label   string
	src     signals.HotSignal
	filters []filter.Compiled
}

func newServiceCmd() *cobra.Command {
	var interval time.Duration
	var bell bool
	c := &cobra.Command{
		Use:   "serve [flight]",
		Short: "Run munin as a long-running service that watches signals in realtime and notifies",
		Long: "Runs munin as a foreground long-running process (not an OS service yet). It\n" +
			"opens each of the flight's signals that supports realtime (a HotSignal),\n" +
			"fans their events into one loop, and prints a notification for each new item.\n\n" +
			"Only Slack is a true websocket (Socket Mode); github/calendar/tasks are polled\n" +
			"at --interval. Signals with no realtime support are skipped. Ctrl-C exits.",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeFlightNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := defaultFlightName()
			if len(args) == 1 {
				name = args[0]
			}
			flight, ok := shared.directives.Flights[name]
			if !ok {
				return errs.Newf(errs.KindUsage, "no flight named %q%s", name, availableFlightSuffix()).
					WithHint("run `munin fly` with no argument to list available flights")
			}
			if !access().FlightVisible(name) {
				return notInRoleError("flight", name)
			}

			jobs := hotJobs(name, flight.Queries, interval)
			if len(jobs) == 0 {
				return errs.Newf(errs.KindUsage, "flight %q has no signals with realtime support", name).
					WithHint("hot signals: slack, github, calendar, tasks, demo")
			}

			ctx := cmd.Context()
			var chans []<-chan signals.Event
			for _, j := range jobs {
				ch, err := j.src.Stream(ctx)
				if err != nil {
					warnf("serve: %s: %v (skipping)", j.label, err)
					continue
				}
				chans = append(chans, applyFilters(ctx, ch, j.filters))
				fmt.Fprintf(cmd.ErrOrStderr(), "watching %-10s %s\n", j.src.Name(), latencyLabel(j.src.LatencyFloor()))
			}
			if len(chans) == 0 {
				return errs.New(errs.KindSignal, "no signals could be opened for watching")
			}

			flightID := shared.audit.StartFlight("serve", shared.cfg.Role)
			defer shared.audit.FinishFlight(flightID)

			events := hot.FanIn(ctx, chans...)
			for {
				select {
				case <-ctx.Done():
					fmt.Fprintln(cmd.ErrOrStderr(), "serve: shutting down")
					return nil
				case ev, ok := <-events:
					if !ok {
						return nil
					}
					shared.audit.RecordQuery(flightID, ev.Source, shared.cfg.Role, ev.At, time.Now(), []signals.Section{ev.Section})
					n, show := mnotify.FromEvent(ev)
					if !show {
						continue
					}
					if bell {
						fmt.Fprint(os.Stdout, "\a")
					}
					fmt.Fprintln(os.Stdout, mnotify.Render(n))
				}
			}
		},
	}
	c.Flags().DurationVar(&interval, "interval", 60*time.Second, "poll interval floor for polled (non-websocket) signals")
	c.Flags().BoolVar(&bell, "bell", true, "ring the terminal bell on each notification")
	return c
}

func hotJobs(flight string, queries []string, interval time.Duration) []hotJob {
	var jobs []hotJob
	for _, name := range queries {
		q, ok := shared.directives.Queries[name]
		if !ok {
			verbosef("serve: unknown query %q in flight %q", name, flight)
			continue
		}
		hs, err := buildHotSignal(q.Signal, hotParams(q.Params, interval))
		if err != nil {
			if errors.Is(err, errNoHotSignal) {
				verbosef("serve: query %q signal %q has no realtime support (skipping)", name, q.Signal)
			} else {
				warnf("serve: query %q: %v (skipping)", name, err)
			}
			continue
		}
		resolved, err := shared.directives.Resolve(q)
		if err != nil {
			warnf("serve: query %q: %v (skipping)", name, err)
			continue
		}
		compiled, err := filter.CompileAll(resolved)
		if err != nil {
			warnf("serve: query %q: %v (skipping)", name, err)
			continue
		}
		jobs = append(jobs, hotJob{label: name, src: hs, filters: compiled})
	}
	return jobs
}

func hotParams(params map[string]string, interval time.Duration) map[string]string {
	out := make(map[string]string, len(params)+1)
	for k, v := range params {
		out[k] = v
	}
	if out["interval"] == "" {
		out["interval"] = interval.String()
	}
	return out
}

func applyFilters(ctx context.Context, in <-chan signals.Event, filters []filter.Compiled) <-chan signals.Event {
	if len(filters) == 0 {
		return in
	}
	out := make(chan signals.Event)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-in:
				if !ok {
					return
				}
				if ev.Section.Err == nil {
					ev.Section.Items = filter.ApplyAll(filters, ev.Section.Items)
					if len(ev.Section.Items) == 0 {
						continue
					}
				}
				select {
				case out <- ev:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out
}

func latencyLabel(d time.Duration) string {
	if d <= 0 {
		return "(push/realtime)"
	}
	return "(~" + d.String() + " poll)"
}

func buildHotSignal(name string, params map[string]string) (signals.HotSignal, error) {
	switch name {
	case "demo":
		return demo.Signal{}, nil
	default:
		return nil, errNoHotSignal
	}
}
