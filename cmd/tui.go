package cmd

import (
	"context"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/audit"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/render"
	"github.com/codyconfer/munin/internal/tui"
	"github.com/codyconfer/munin/internal/views"
	"github.com/codyconfer/sisyphus"
	sconfig "github.com/codyconfer/sisyphus/config"
)

func newTuiCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "tui [flight]",
		Short:             "Open the cyberpunk TUI (main menu, or a flight directly)",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeFlightNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !term.IsTerminal(int(os.Stdout.Fd())) {
				return errs.New(errs.KindUsage, "tui requires an interactive terminal")
			}
			kit := buildViews()
			if len(args) == 1 {
				name := args[0]
				if _, ok := shared.directives.Flights[name]; !ok {
					return errs.Newf(errs.KindUsage, "no flight named %q%s", name, availableFlightSuffix())
				}
				return tui.Run(kit.FlightResults(name))
			}
			return tui.Run(kit.MainMenu())
		},
	}
}

func buildViews() *views.Kit {
	return views.New(views.Deps{
		Home:           func() string { return shared.cfg.Home },
		Role:           func() string { return access().Role },
		Directives:     func() *config.Directives { return shared.directives },
		Config:         func() *config.Config { return shared.cfg },
		Mgr:            func() *sisyphus.Manager { return shared.mgr },
		Audit:          func() *audit.Store { return shared.audit },
		VisibleFlights: visibleFlightNames,
		RunQuery:       runQueryBody,
		RunFlight:      runFlightBody,
		RunFlightAudited: func(name string) string {
			return runFlightAuditedBody(name)
		},
		Verify:           verifyFindings,
		ExportDirectives: exportDirectivesToFiles,
	})
}

func runQueryBody(name string) string {
	j, err := buildQueryJob(name)
	if err != nil {
		return theme.Cur().Cant.Render("error: " + err.Error())
	}
	return render.RenderTerminalString(fetchJobs(context.Background(), []job{j}, 0))
}

func runFlightBody(name string) string {
	fl := shared.directives.Flights[name]
	return render.RenderTerminalString(fetchJobs(context.Background(), flightJobs(name, fl.Queries), 0))
}

func runFlightAuditedBody(name string) string {
	fl := shared.directives.Flights[name]
	jobs := flightJobs(name, fl.Queries)
	fid := shared.audit.StartFlight(name, shared.cfg.Role)
	sections := fetchJobs(context.Background(), jobs, fid)
	shared.audit.FinishFlight(fid)
	return render.RenderTerminalString(sections)
}

func verifyFindings(kind string) []views.Finding {
	var raw []finding
	switch kind {
	case "queries":
		raw = verifyQueries()
	case "flights":
		raw = verifyFlights()
	case "roles":
		raw = verifyRoles()
	}
	out := make([]views.Finding, 0, len(raw))
	for _, f := range raw {
		out = append(out, views.Finding{Name: f.name, Msg: f.msg, OK: f.ok, Warn: f.warn})
	}
	return out
}

func exportDirectivesToFiles() ([]string, error) {
	if shared.mgr == nil {
		return nil, errs.New(errs.KindInternal, "config DB unavailable")
	}
	home := shared.cfg.Home
	db := shared.mgr.DB()
	var written []string

	if cur, ok, err := db.Current("config"); err != nil {
		return nil, err
	} else if ok {
		path, err := sconfig.WriteConfigFile(home, []byte(cur.Content), cur.Format)
		if err != nil {
			return nil, err
		}
		written = append(written, path)
	}

	for _, name := range []string{config.DirQueries, config.DirFilters, config.DirFlights, config.DirRoles} {
		cur, ok, err := db.Current(name)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		dir := filepath.Join(home, name)
		names, err := sconfig.WriteCollection(dir, []byte(cur.Content))
		if err != nil {
			return nil, err
		}
		for _, fn := range names {
			written = append(written, filepath.Join(dir, fn))
		}
	}
	return written, nil
}
