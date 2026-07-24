package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	sysdaemon "github.com/codyconfer/sisyphus/daemon"

	"github.com/codyconfer/munin/internal/app/loginflow"
	"github.com/codyconfer/munin/internal/app/onboard"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/log"
	"github.com/codyconfer/munin/internal/render/glyph"
	gh "github.com/codyconfer/munin/internal/signals/github"
)

const annoGateMode = "munin_gate_mode"

const (
	modeCLI    = "cli"
	modeServe  = "serve"
	modeDaemon = "daemon"
	modeDeck   = "deck"
)

type authState int

const (
	authUnauthenticated authState = iota
	authUnauthorized
	authAuthorized
)

type installState int

const (
	installNotInstalled installState = iota
	installStopped
	installRunning
)

func gateMode(cmd *cobra.Command) string {
	for c := cmd; c != nil; c = c.Parent() {
		if m, ok := c.Annotations[annoGateMode]; ok && m != "" {
			return m
		}
	}
	return modeCLI
}

func gate(cmd *cobra.Command) error {
	if skipsOnboarding(cmd) {
		return nil
	}
	a := classifyAuth(cmd.Context())
	switch gateMode(cmd) {
	case modeServe:
		return gateServe(cmd, a)
	case modeDaemon:
		return gateDaemon(cmd, a)
	case modeDeck:
		return gateDeck(cmd, a)
	default:
		return gateCLI(cmd, a)
	}
}

func classifyAuth(ctx context.Context) authState {
	gs := config.LoadGlobalSettings()
	if gs.Onboarded && gs.OnboardedDomain == onboard.RequiredEmailDomain {
		return authAuthorized
	}
	if !shared.GitHubAuthed() {
		return authUnauthenticated
	}
	apiURL, _ := gh.NormalizeAPIURL(shared.Cfg.GitHub.APIURL)
	if onboard.Check(ctx, shared.Tokens, apiURL).Ready() {
		return authAuthorized
	}
	return authUnauthorized
}

func classifyInstall() installState {
	srv := serveServer()
	if sysdaemon.IsListening(srv.SocketPath()) {
		return installRunning
	}
	svc, err := srv.Service(defaultFlightName(), configServeInterval(), shared.Cfg.Daemon.Bell, shared.Cfg.Daemon.Desktop, configServeTheme(), true)
	if err != nil {
		return installNotInstalled
	}
	st, sErr := svc.Status()
	switch {
	case sErr != nil:
		return installNotInstalled
	case st == "running":
		return installRunning
	default:
		return installStopped
	}
}

func gateCLI(cmd *cobra.Command, a authState) error {
	switch a {
	case authUnauthenticated:
		return cliGuidedAuth(cmd)
	case authUnauthorized:
		if onboard.EnforceAuthorized == "true" {
			return errs.New(errs.KindOnboarding, "munin is not fully authorized").WithHint("%s", onboardHint())
		}
		gateWarn(cmd, onboardHint())
		return nil
	default:
		return nil
	}
}

func cliGuidedAuth(cmd *cobra.Command) error {
	p, ok := loginflow.Resolve("github")
	if !ok {
		return errs.New(errs.KindInternal, "github login provider unavailable")
	}
	if err := runLogin(cmd, p); err != nil {
		return err
	}
	shared.ResetGitHubAuth()
	return runOnboard(cmd, false)
}

func gateServe(cmd *cobra.Command, a authState) error {
	if a != authAuthorized {
		gateWarn(cmd, "serve: "+onboardHint())
	}
	if sysdaemon.IsListening(serveServer().SocketPath()) {
		gateWarn(cmd, "serve: a munin daemon is already listening; this instance will not own the socket")
	}
	return nil
}

func gateDaemon(_ *cobra.Command, a authState) error {
	if a != authAuthorized {
		log.Warnf("daemon: %s", onboardHint())
	}
	return nil
}

func gateDeck(cmd *cobra.Command, _ authState) error {
	return requireOnboarding(cmd)
}

func gateWarn(cmd *cobra.Command, msg string) {
	fmt.Fprintf(cmd.ErrOrStderr(), "%s %s\n", glyph.Warn(), msg)
}

func daemonServiceStatus() deck.ServiceStatus {
	switch classifyInstall() {
	case installRunning:
		return deck.ServiceStatus{Name: "daemon", Detail: "running", Level: deck.StatusOK}
	case installStopped:
		return deck.ServiceStatus{Name: "daemon", Detail: "stopped", Level: deck.StatusWarn}
	default:
		return deck.ServiceStatus{Name: "daemon", Detail: "not installed", Level: deck.StatusMuted}
	}
}
