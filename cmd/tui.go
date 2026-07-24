package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/app/onboard"
	"github.com/codyconfer/munin/internal/app/verify"
	"github.com/codyconfer/munin/internal/app/views"
	"github.com/codyconfer/munin/internal/auth"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/log"
	"github.com/codyconfer/munin/internal/signals"
	gh "github.com/codyconfer/munin/internal/signals/github"
	muninterm "github.com/codyconfer/munin/internal/term"
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
			if !term.IsTerminal(os.Stdout.Fd()) {
				return errs.New(errs.KindUsage, "deck requires an interactive terminal")
			}
			ensureLiveProvider(cmd, args)
			kit := buildViews()
			if len(args) == 1 {
				name := args[0]
				if _, ok := shared.Directives.Flights[name]; !ok {
					return errs.Newf(errs.KindUsage, "no flight named %q%s", name, availableFlightSuffix())
				}
				return deck.Run(kit.FlightResults(name), deck.WithStatus(statusProvider()))
			}
			return deck.Run(kit.Home(), deck.WithStatus(statusProvider()))
		},
	}
}

func ensureLiveProvider(cmd *cobra.Command, args []string) {
	srv := serveServer()
	if _, ok := srv.Dial(cmd.Context()); ok {
		return
	}
	self, err := muninterm.Self()
	if err != nil {
		log.Debugf("deck: cannot locate munin binary to start a serve provider: %v", err)
		return
	}
	name := defaultFlightName()
	if len(args) == 1 {
		name = args[0]
	}
	if err := muninterm.Open([]string{self, "serve", name}); err != nil {
		log.Debugf("deck: could not open a serve provider window: %v", err)
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
	fl := shared.Directives.Flights[name]
	queries := flightQueries(name, fl.Queries)
	fid := shared.Audit.StartFlight(name, shared.Cfg.Role)
	sections := fetchQueries(context.Background(), queries, fid)
	shared.Audit.FinishFlight(fid)
	return sections
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
	return func(ctx context.Context) deck.StatusInfo {
		apiURL, _ := gh.NormalizeAPIURL(shared.Cfg.GitHub.APIURL)
		var info deck.StatusInfo

		user, rate, ghOK := githubStatus(ctx, apiURL)
		info.GitHubUser = user
		info.Services = append(info.Services, rate)

		if ghOK {
			st := onboard.Check(ctx, shared.Tokens, apiURL)
			info.SigningVerified = signingVerified(st)
		}

		slackLevel := deck.StatusMuted
		if _, err := auth.SlackToken(shared.Tokens, ""); err == nil {
			slackLevel = deck.StatusOK
		}
		info.Services = append(info.Services, deck.ServiceStatus{Name: "slack", Level: slackLevel})

		googleLevel := deck.StatusMuted
		if auth.GoogleAuthed(shared.Tokens) {
			googleLevel = deck.StatusOK
		}
		for _, name := range []string{"calendar", "gmail", "docs", "drive", "tasks"} {
			info.Services = append(info.Services, deck.ServiceStatus{Name: name, Level: googleLevel})
		}
		info.Services = append(info.Services, daemonServiceStatus())
		return info
	}
}

func githubStatus(ctx context.Context, apiURL string) (user string, svc deck.ServiceStatus, ok bool) {
	svc = deck.ServiceStatus{Name: "github"}
	raw, err := auth.GHAPIGet(ctx, shared.Tokens, apiURL, "user")
	if err != nil {
		svc.Level = deck.StatusBad
		return "", svc, false
	}
	var u struct {
		Login string `json:"login"`
	}
	_ = json.Unmarshal(raw, &u)

	limit, remaining, rateOK := githubRate(ctx, apiURL)
	if !rateOK {
		svc.Level = deck.StatusOK
		return u.Login, svc, true
	}
	svc.Detail = fmt.Sprintf("%d/%d", remaining, limit)
	switch {
	case remaining == 0:
		svc.Level = deck.StatusBad
	case remaining*5 < limit:
		svc.Level = deck.StatusWarn
	default:
		svc.Level = deck.StatusOK
	}
	return u.Login, svc, true
}

func githubRate(ctx context.Context, apiURL string) (limit, remaining int, ok bool) {
	raw, err := auth.GHAPIGet(ctx, shared.Tokens, apiURL, "rate_limit")
	if err != nil {
		return 0, 0, false
	}
	var r struct {
		Resources struct {
			Core struct {
				Limit     int `json:"limit"`
				Remaining int `json:"remaining"`
			} `json:"core"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(raw, &r); err != nil || r.Resources.Core.Limit == 0 {
		return 0, 0, false
	}
	return r.Resources.Core.Limit, r.Resources.Core.Remaining, true
}

func signingVerified(st onboard.Status) bool {
	for _, r := range st.Results {
		if r.Step == onboard.StepGPGGitHub || r.Step == onboard.StepSSHGitHub {
			return r.OK
		}
	}
	return false
}
