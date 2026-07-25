package cmd

import (
	"context"
	"os"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/app/statusstrip"
	"github.com/codyconfer/munin/internal/app/verify"
	"github.com/codyconfer/munin/internal/app/views"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/signals"
)

func newDeckCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "deck [flight]",
		Aliases:           []string{"tui"},
		Short:             "Open the cyberpunk TUI (main menu, or a flight directly)",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeFlightNames,
		Annotations:       map[string]string{annoGateMode: modeDeck},
		RunE: func(cmd *cobra.Command, args []string) error {
			defer stopLaunchLoading()
			if !term.IsTerminal(os.Stdout.Fd()) {
				return errs.New(errs.KindUsage, "deck requires an interactive terminal")
			}
			name := defaultFlightName()
			if len(args) == 1 {
				name = args[0]
				if _, ok := shared.Directives.Flights[name]; !ok {
					return errs.Newf(errs.KindUsage, "no flight named %q%s", name, availableFlightSuffix())
				}
			}
			stopServe := ensureServeProvider(cmd.Context(), name)
			defer stopServe()
			kit := buildViews()
			stopLaunchLoading()
			opts := []deck.Option{deck.WithStatus(statusProvider()), deck.WithKeyHook(kit.KeyHook())}
			if len(args) == 1 {
				return deck.Run(kit.FlightResults(name), opts...)
			}
			return deck.Run(kit.Home(), opts...)
		},
	}
}

func buildViews() *views.Kit {
	return views.New(views.Deps{
		App:                shared,
		FetchQuery:         fetchQuerySections,
		FetchFlight:        fetchFlightSections,
		FetchFlightAudited: fetchFlightAuditedSections,
		FetchHomeFlight:    fetchHomeFlightSections,
		Verify:             verifyFindings,
		ExportDirectives:   exportDirectivesToFiles,
	})
}

func fetchQuerySections(name string) []signals.Section {
	q, err := buildQuery(name)
	if err != nil {
		return []signals.Section{{Signal: name, Title: name, Err: err}}
	}
	return fetchQueries(context.Background(), []query{q}, 0)
}

func fetchFlightSections(name string) []signals.Section {
	fl := shared.Directives.Flights[name]
	return fetchQueries(context.Background(), flightQueries(name, fl.Queries), 0)
}

func fetchFlightAuditedSections(name string) []signals.Section {
	fl := shared.Directives.Flights[name]
	queries := flightQueries(name, fl.Queries)
	fid := shared.Audit.StartFlight(name, shared.Cfg.Role)
	sections := fetchQueries(context.Background(), queries, fid)
	shared.Audit.FinishFlight(fid)
	return sections
}

func fetchHomeFlightSections(name string) []signals.Section {
	return fetchFlightAuditedSections(name)
}

func verifyFindings(kind string) []verify.Finding {
	switch kind {
	case "queries":
		return verify.Queries(shared.Directives)
	case "flights":
		return verify.Flights(shared.Directives)
	case "roles":
		return verify.Roles(shared.Directives)
	}
	return nil
}

func exportDirectivesToFiles() ([]string, error) {
	if shared.Mgr == nil {
		return nil, errs.New(errs.KindInternal, "config DB unavailable")
	}
	return config.ExportAllToFiles(shared.Mgr.DB(), shared.Cfg.Home)
}

func statusProvider() deck.StatusFunc {
	return statusstrip.Provider(shared, daemonStatusChip())
}
