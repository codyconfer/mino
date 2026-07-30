package cmd

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/codyconfer/viewkit/clipboard"
	muninterm "github.com/codyconfer/viewkit/term"

	sconfig "github.com/codyconfer/sisyphus/config"

	"github.com/codyconfer/munin/internal/app/pane"
	"github.com/codyconfer/munin/internal/app/statusstrip"
	"github.com/codyconfer/munin/internal/app/verify"
	"github.com/codyconfer/munin/internal/app/views"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/signals"
	"github.com/codyconfer/munin/internal/signals/build"
	"github.com/codyconfer/munin/internal/tmux"
)

func newDeckCmd() *cobra.Command {
	var useTmux bool
	c := &cobra.Command{
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
			name := deckFlightName()
			if len(args) == 1 {
				name = args[0]
				if _, ok := shared.Directives.Flights[name]; !ok {
					return errs.Newf(errs.KindUsage, "no flight named %q%s", name, availableFlightSuffix())
				}
			}
			if useTmux && !tmux.Inside() {
				stopLaunchLoading()
				return launchTmuxDeck(args)
			}
			panes, err := deckPanes(useTmux, name)
			if err != nil {
				return err
			}
			defer panes.CloseAll()

			stopServe := ensureServeProvider(cmd.Context(), name)
			defer stopServe()
			kit := buildViewsWithPanes(panes)
			stopLaunchLoading()
			opts := []deck.Option{
				deck.WithStatus(statusProvider()),
				deck.WithKeyHook(kit.KeyHook()),
				deck.WithMsgHook(kit.MsgHook()),
				deck.WithInitCmd(views.StoreTick()),
			}
			if len(args) == 1 {
				return deck.Run(kit.FlightResults(name), opts...)
			}
			return deck.Run(kit.Home(), opts...)
		},
	}
	c.Flags().BoolVar(&useTmux, "tmux", false, "open the deck inside a tmux session so it can split off auxiliary panes")
	return c
}

func launchTmuxDeck(args []string) error {
	if !tmux.Available() {
		return errs.New(errs.KindUsage, "--tmux needs tmux on PATH").
			WithHint("install tmux, or run `munin deck` without --tmux")
	}
	self, err := muninterm.Self()
	if err != nil {
		return errs.Wrap(errs.KindInternal, err, "locate munin binary")
	}
	argv := append([]string{"deck"}, args...)
	argv = append(argv, "--tmux")
	if flagHome != "" {
		argv = append(argv, "--home", flagHome)
	}
	return tmux.Launch(self, argv)
}

func deckFlightName() string {
	if rd, ok := shared.Directives.Roles[shared.Cfg.Role]; ok && rd.Home != "" {
		if _, exists := shared.Directives.Flights[rd.Home]; exists {
			return rd.Home
		}
	}
	return defaultFlightName()
}

func deckPanes(useTmux bool, flight string) (*pane.Manager, error) {
	if !useTmux {
		return nil, nil
	}
	if !tmux.Available() {
		return nil, errs.New(errs.KindUsage, "--tmux needs tmux on PATH").
			WithHint("install tmux, or run `munin deck` without --tmux")
	}
	pane.CleanupSnapshots(shared.Cfg.Home)
	return pane.NewManager(shared.Cfg.Home, flight)
}

func buildViews() *views.Kit { return buildViewsWithPanes(nil) }

func buildViewsWithPanes(panes *pane.Manager) *views.Kit {
	return views.New(views.Deps{
		App:                shared,
		Panes:              panes,
		FetchQuery:         fetchQuerySections,
		FetchFlightAudited: fetchFlightAuditedSections,
		FetchHomeFlight:    fetchHomeFlightSections,
		FetchAdhoc:         fetchAdhocSections,
		FetchFlightQueries: fetchFlightQueriesSections,
		FetchDetail:        fetchItemDetail,
		Verify:             verifyFindings,
		ExportDirectives:   exportDirectivesToFiles,
		RenderReport:       renderSectionsReport,
		CopyText:           clipboard.Copy,
		SaveReport:         saveReportFile,
		PreviewRole:        shared.PreviewRole,
	})
}

func renderSectionsReport(fd config.FormatterDef, label string, sections []signals.Section) (string, error) {
	return renderReport(fd, label, "flight", []flightGroup{{Query: label, Title: label, Sections: sections}})
}

func saveReportFile(formatter, text string) (string, error) {
	dir := filepath.Join(shared.Cfg.Home, config.DirReports)
	name := formatter + "-" + time.Now().Format("20060102-150405") + ".md"
	path, err := sconfig.WriteItem(dir, name, []byte(text))
	if err != nil {
		return "", errs.Wrapf(errs.KindConfig, err, "writing report to %s", dir)
	}
	return path, nil
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
	case "formatters":
		return verify.Formatters(shared.Directives)
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
