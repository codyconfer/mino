package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/render"
	"github.com/codyconfer/mino/internal/render/glyph"
)

const defaultFlight = "default"

func newFlyCmd() *cobra.Command {
	var ff formatterFlags
	c := &cobra.Command{
		Use:   "fly [flight]",
		Short: "Send mino flying — run a named flight (a configured set of queries)",
		Long: "Flights live one-per-file under flights/ as named lists of saved query\n" +
			"names. `mino fly <flight>` runs that flight's queries, in order. With no\n" +
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
			if _, ok := shared.Directives.Flights[name]; !ok && len(args) == 0 {
				return listFlights(cmd)
			}
			o, err := ff.resolve(cmd, shared.Directives.Flights[name].Formatter)
			if err != nil {
				return err
			}
			return runFlightNamed(cmd, name, o)
		},
	}
	ff.bind(c)
	return c
}

func runFlightNamed(cmd *cobra.Command, name string, o runOpts) error {
	fl, ok := shared.Directives.Flights[name]
	if !ok {
		return errs.Newf(errs.KindUsage, "no flight named %q%s", name, availableFlightSuffix()).WithHint("run `mino fly` with no argument to list available flights")
	}
	if !access().FlightVisible(name) {
		return notInRoleError("flight", name)
	}
	if len(fl.Queries) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "flight %q has no queries\n", name)
		return nil
	}

	queries := flightQueries(name, fl.Queries)

	flightID := shared.Audit.StartFlight(name, shared.Cfg.Role)
	defer shared.Audit.FinishFlight(flightID)

	return runQueriesWith(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), name, queries, flightID, o)
}

func flightQueries(flight string, names []string) []query {
	out := make([]query, 0, len(names))
	for _, name := range names {
		q, err := buildQuery(name)
		if err != nil {
			verbosef("flight %q: %v", flight, err)
			title := shared.Directives.Queries[name].Display()
			out = append(out, query{Label: name, Title: title, Src: errSignal{name: name, err: err}})
			continue
		}
		out = append(out, q)
	}
	return out
}

func defaultFlightName() string {
	if rd, ok := shared.Directives.Roles[shared.Cfg.Role]; ok && len(rd.Flights) > 0 {
		return rd.Flights[0]
	}
	return defaultFlight
}

func interactiveTTY() bool {
	return render.Format(shared.Cfg.Output) == render.FormatTerminal &&
		term.IsTerminal(os.Stdout.Fd())
}

func listFlights(cmd *cobra.Command) error {
	names := visibleFlightNames()
	if len(names) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(),
			"no flights visible; define them under `flights:` in config.yaml (see `mino fly --help`)")
		return nil
	}
	fmt.Fprintln(cmd.OutOrStdout(), "available flights:")
	marker := theme.Cur().Accent.Render(glyph.Flight())
	for _, n := range names {
		fmt.Fprintf(cmd.OutOrStdout(), "%s %-16s %s\n", marker, n, strings.Join(shared.Directives.Flights[n].Queries, ", "))
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
