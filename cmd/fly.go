package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/render"
	"github.com/codyconfer/munin/internal/signals"
	"github.com/codyconfer/munin/internal/ui"
)

const defaultFlight = "default"

func newFlyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "fly [flight]",
		Short: "Send munin flying — run a named flight (a configured set of queries)",
		Long: "Flights live one-per-file under flights/ as named lists of saved query\n" +
			"names. `munin fly <flight>` runs that flight's queries, in order. With no\n" +
			"name, it runs the active role's first flight (or \"default\"); if that is\n" +
			"undefined it lists the available flights. The active role determines which\n" +
			"flights and queries are visible.",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeFlightNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := defaultFlightName()
			if len(args) == 1 {
				name = args[0]
			}

			flight, ok := shared.directives.Flights[name]
			if !ok {
				if len(args) == 0 {
					return listFlights(cmd)
				}
				return errs.Newf(errs.KindUsage, "no flight named %q%s", name, availableFlightSuffix()).WithHint("run `munin fly` with no argument to list available flights")
			}
			if !access().FlightVisible(name) {
				return notInRoleError("flight", name)
			}
			if len(flight.Queries) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "flight %q has no queries\n", name)
				return nil
			}

			jobs := flightJobs(name, flight.Queries)

			flightID := shared.audit.StartFlight(name, shared.cfg.Role)
			defer shared.audit.FinishFlight(flightID)

			if interactiveTTY() {
				tasks := make([]ui.Task, len(jobs))
				for i, j := range jobs {
					j := j
					tasks[i] = ui.Task{
						Label: j.label,
						Run:   func(ctx context.Context) []signals.Section { return fetchOne(ctx, j, flightID) },
					}
				}
				return ui.RunFlight(cmd.Context(), tasks)
			}
			return emit(fetchJobs(cmd.Context(), jobs, flightID))
		},
	}
}

func flightJobs(flight string, queries []string) []job {
	jobs := make([]job, 0, len(queries))
	for _, q := range queries {
		j, err := buildQueryJob(q)
		if err != nil {
			verbosef("flight %q: %v", flight, err)
			jobs = append(jobs, job{label: q, src: errSignal{name: q, err: err}})
			continue
		}
		jobs = append(jobs, j)
	}
	return jobs
}

func defaultFlightName() string {
	if rd, ok := shared.directives.Roles[shared.cfg.Role]; ok && len(rd.Flights) > 0 {
		return rd.Flights[0]
	}
	return defaultFlight
}

func interactiveTTY() bool {
	return render.Format(shared.cfg.Output) == render.FormatTerminal &&
		term.IsTerminal(int(os.Stdout.Fd()))
}

func listFlights(cmd *cobra.Command) error {
	names := visibleFlightNames()
	if len(names) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(),
			"no flights visible; define them under `flights:` in config.yaml (see `munin fly --help`)")
		return nil
	}
	fmt.Fprintln(cmd.OutOrStdout(), "available flights:")
	for _, n := range names {
		fmt.Fprintf(cmd.OutOrStdout(), "  %-16s %s\n", n, strings.Join(shared.directives.Flights[n].Queries, ", "))
	}
	return nil
}

func availableFlightSuffix() string {
	names := visibleFlightNames()
	if len(names) == 0 {
		return " (no flights visible)"
	}
	return " (available: " + strings.Join(names, ", ") + ")"
}

func completeFlightNames(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return visibleFlightNames(), cobra.ShellCompDirectiveNoFileComp
}
