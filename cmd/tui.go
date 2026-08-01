package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/codyconfer/viewkit/clipboard"
	vkdeck "github.com/codyconfer/viewkit/deck"

	sconfig "github.com/codyconfer/sisyphus/config"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/app/pane"
	"github.com/codyconfer/mino/internal/app/statusstrip"
	"github.com/codyconfer/mino/internal/app/verify"
	"github.com/codyconfer/mino/internal/app/views"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/deck"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/signals"
	"github.com/codyconfer/mino/internal/signals/build"
	"github.com/codyconfer/mino/internal/tmux"
)

var (
	confirmDeckInstall = deck.Confirm
	installDeckHome    = app.Install
)

func installDeckExamples(cmd *cobra.Command) error {
	if flagConfigFile != "" {
		return nil
	}
	home, err := config.Home(flagHome)
	if err != nil {
		return err
	}
	needed, err := app.NeedsInstall(home)
	if err != nil || !needed {
		return err
	}
	ok, err := confirmDeckInstall(vkdeck.ConfirmSpec{
		Title:    "Install example config?",
		Message:  fmt.Sprintf("No config or directives were found in %s. Run mino install now to generate the example config, queries, flights, and role?", home),
		YesLabel: "Install",
		NoLabel:  "Continue empty",
	})
	if err != nil || !ok {
		return err
	}
	created, err := installDeckHome(home, false)
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.ErrOrStderr(), Scope().Success(fmt.Sprintf("installed %d example files and stores in %s", len(created), home)))
	return nil
}

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
				if _, ok := shared.Dirs().Flights[name]; !ok {
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
			kit := buildViewsFor(cmd.Context(), panes)
			stopLaunchLoading()
			opts := []deck.Option{
				deck.WithScope(Scope()),
				deck.WithStatus(Scope(), statusProvider()),
				deck.WithKeyHook(kit.KeyHook()),
				deck.WithMsgHook(kit.MsgHook()),
				deck.WithInitCmd(views.StoreTick()),
			}
			if len(args) == 1 {
				return deck.RunContext(kit.FlightResults(name), consoleRole(), opts...)
			}
			return deck.RunContext(kit.Home(), consoleRole(), opts...)
		},
	}
	c.Flags().BoolVar(&useTmux, "tmux", false, "open the deck inside a tmux session so it can split off auxiliary panes")
	return c
}

func consoleRole() string {
	if role := shared.Role(); role != "" {
		return "role: " + role
	}
	return ""
}

func launchTmuxDeck(args []string) error {
	if !tmux.Available() {
		return errs.New(errs.KindUsage, "--tmux needs tmux on PATH").
			WithHint("install tmux, or run `mino deck` without --tmux")
	}
	self, err := os.Executable()
	if err != nil {
		return errs.Wrap(errs.KindInternal, err, "locate mino binary")
	}
	argv := append([]string{"deck"}, args...)
	argv = append(argv, "--tmux")
	if flagHome != "" {
		argv = append(argv, "--home", flagHome)
	}
	return tmux.Launch(self, argv)
}

func deckFlightName() string {
	d := shared.Dirs()
	if rd, ok := d.Roles[shared.Role()]; ok && rd.Home != "" {
		if _, exists := d.Flights[rd.Home]; exists {
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
			WithHint("install tmux, or run `mino deck` without --tmux")
	}
	pane.CleanupSnapshots(shared.Cfg.Home)
	return pane.NewManager(shared.Cfg.Home, flight)
}

func buildViews() *views.Kit { return buildViewsFor(context.Background(), nil) }

func buildViewsFor(ctx context.Context, panes *pane.Manager) *views.Kit {
	if ctx == nil {
		ctx = context.Background()
	}
	return views.New(views.Deps{
		App:   shared,
		Panes: panes,
		Scope: Scope(),
		FetchQuery: func(name string) []signals.Section {
			return fetchQuerySections(ctx, name)
		},
		FetchFlightAudited: func(name string) []signals.Section {
			return fetchFlightAuditedSections(ctx, name)
		},
		FetchHomeFlight: func(name string) []signals.Section {
			return fetchFlightAuditedSections(ctx, name)
		},
		FetchAdhoc: func(q config.Query) []signals.Section {
			return fetchAdhocSections(ctx, q)
		},
		FetchFlightQueries: func(label string, names []string) []signals.Section {
			return fetchFlightQueriesSections(ctx, label, names)
		},
		FetchDetail: func(signal string, it signals.Item) (*signals.ItemDetail, error) {
			return fetchItemDetail(ctx, signal, it)
		},
		Verify:           verifyFindings,
		ExportDirectives: exportDirectivesToFiles,
		RenderReport:     renderSectionsReport,
		CopyText:         clipboard.Copy,
		SaveReport:       saveReportFile,
		PreviewRole:      shared.PreviewRole,
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

func fetchItemDetail(ctx context.Context, signal string, it signals.Item) (*signals.ItemDetail, error) {
	ctx, cancel := context.WithTimeout(ctx, sourceTimeout())
	defer cancel()
	d, err := build.Detail(ctx, signal, it, shared.Cfg, shared.Tokens, shared.Cache)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func fetchAdhocSections(ctx context.Context, q config.Query) []signals.Section {
	label := q.Name
	if label == "" {
		label = "ad-hoc"
	}
	built, err := buildQueryFrom(label, q)
	if err != nil {
		return []signals.Section{{Signal: q.Signal, Title: label, Err: err}}
	}
	return fetchQueries(ctx, []query{built}, 0)
}

func fetchQuerySections(ctx context.Context, name string) []signals.Section {
	q, err := buildQuery(name)
	if err != nil {
		return []signals.Section{{Signal: name, Title: name, Err: err}}
	}
	return fetchQueries(ctx, []query{q}, 0)
}

func fetchFlightAuditedSections(ctx context.Context, name string) []signals.Section {
	return fetchFlightQueriesSections(ctx, name, shared.Dirs().Flights[name].Queries)
}

func fetchFlightQueriesSections(ctx context.Context, label string, names []string) []signals.Section {
	queries := flightQueries(label, names)
	fid := shared.Audit.StartFlightContext(ctx, label, shared.Role())
	sections := fetchQueries(ctx, queries, fid)
	shared.Audit.FinishFlight(fid)
	return sections
}

func verifyFindings(kind string) []verify.Finding {
	d := shared.Dirs()
	switch kind {
	case "queries":
		return verify.Queries(d)
	case "flights":
		return verify.Flights(d)
	case "roles":
		return verify.Roles(d)
	case "formatters":
		return verify.Formatters(d)
	}
	return nil
}

func exportDirectivesToFiles() ([]string, error) {
	if shared.Mgr == nil {
		return nil, errs.New(errs.KindInternal, "config DB unavailable")
	}
	return config.ExportAllToFiles(shared.Mgr, shared.Cfg.Home)
}

func statusProvider() deck.StatusFunc {
	return statusstrip.Provider(shared)
}
