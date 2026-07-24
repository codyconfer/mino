package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	sysdaemon "github.com/codyconfer/sisyphus/daemon"
	"github.com/codyconfer/sisyphus/mode"

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
	modeCLI    = string(mode.ModeCLI)
	modeServe  = string(mode.ModeServe)
	modeDaemon = string(mode.ModeDaemon)
	modeDeck   = string(mode.ModeDeck)
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
	m := mode.Mode(gateMode(cmd))
	err := mode.Gate(cmd.Context(), m, mode.GateHooks{
		Classify:           classifyAuth,
		CLIUnauthenticated: func(ctx context.Context) error { return cliGuidedAuth(cmd) },
		CLIUnauthorized: func(ctx context.Context) error {
			if onboard.EnforceAuthorized == "true" {
				return errs.New(errs.KindOnboarding, "munin is not fully authorized").WithHint("%s", onboardHint())
			}
			gateWarn(cmd, onboardHint())
			return nil
		},
		EnforceHard: onboard.EnforceAuthorized == "true",
		ServeUnauthorized: func(ctx context.Context) error {
			gateWarn(cmd, "serve: "+onboardHint())
			return nil
		},
		DaemonUnauthorized: func(ctx context.Context) error {
			log.Warnf("daemon: %s", onboardHint())
			return nil
		},
		DeckRequire: func(ctx context.Context) error {
			return requireOnboarding(cmd)
		},
	})
	if err != nil {
		return err
	}
	// Serve always checks for an existing socket owner (auth-independent).
	if m == mode.ModeServe && sysdaemon.IsListening("munin", serveServer().SocketPath()) {
		gateWarn(cmd, "serve: a munin daemon is already listening; this instance will not own the socket")
	}
	return nil
}

func classifyAuth(ctx context.Context) mode.AuthState {
	gs := config.LoadGlobalSettings()
	if gs.Onboarded && gs.OnboardedDomain == onboard.RequiredEmailDomain {
		return mode.AuthAuthorized
	}
	if !shared.GitHubAuthed() {
		return mode.AuthUnauthenticated
	}
	apiURL, _ := gh.NormalizeAPIURL(shared.Cfg.GitHub.APIURL)
	if onboard.Check(ctx, shared.Tokens, apiURL).Ready() {
		return mode.AuthAuthorized
	}
	return mode.AuthUnauthorized
}

func classifyInstall() installState {
	srv := serveServer()
	if sysdaemon.IsListening("munin", srv.SocketPath()) {
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
