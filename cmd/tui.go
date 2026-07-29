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
	"github.com/codyconfer/munin/internal/signals/build"
)

func newDeckCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "deck [flight]",
		Aliases:           []string{"tui"},
		Short:             "Open the cyberpunk TUI (main menu, or a flight directly)",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeFlightNames,
		Annotations: map[string]string{
			annoGateMode:      modeDeck,
			AnnoLaunchLoading: "true",
		},
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
			opts := []deck.Option{
				deck.WithStatus(statusProvider()),
				deck.WithKeyHook(kit.KeyHook()),
				deck.WithMsgHook(kit.MsgHook()),
			}
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
		FetchFlightAudited: fetchFlightAuditedSections,
		FetchHomeFlight:    fetchHomeFlightSections,
		FetchAdhoc:         fetchAdhocSections,
		FetchFlightQueries: fetchFlightQueriesSections,
		FetchDetail:        fetchItemDetail,
		Verify:             verifyFindings,
		ExportDirectives:   exportDirectivesToFiles,
	})
}

func fetchItemDetail(signal string, it signals.Item) (*signals.ItemDetail, error) {
	ctx, cancel := context.WithTimeout(context.Background(), sourceTimeout())
	defer cancel()
	d, err := build.Detail(ctx, signal, it, shared.Cfg, shared.Tokens, shared.Cache)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func fetchAdhocSections(q config.Query) []signals.Section {
	label := q.Name
	if label == "" {
		label = "ad-hoc"
	}
	built, err := buildQueryFrom(label, q)
	if err != nil {
		return []signals.Section{{Signal: q.Signal, Title: label, Err: err}}
	}
	return fetchQueries(context.Background(), []query{built}, 0)
}

func fetchQuerySections(name string) []signals.Section {
	q, err := buildQuery(name)
	if err != nil {
		return []signals.Section{{Signal: name, Title: name, Err: err}}
	}
	return fetchQueries(context.Background(), []query{q}, 0)
}

func fetchFlightAuditedSections(name string) []signals.Section {
	return fetchFlightQueriesSections(name, shared.Directives.Flights[name].Queries)
}

func fetchFlightQueriesSections(label string, names []string) []signals.Section {
	queries := flightQueries(label, names)
	fid := shared.Audit.StartFlight(label, shared.Cfg.Role)
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
	return statusstrip.Provider(shared)
}
