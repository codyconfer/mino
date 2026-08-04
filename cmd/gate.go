package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/codyconfer/sisyphus/mode"

	"github.com/codyconfer/mino/internal/app/loginflow"
	"github.com/codyconfer/mino/internal/app/onboard"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
)

const annoGateMode = "mino_gate_mode"

const annoThin = "mino_thin"

const (
	modeCLI    = string(mode.ModeCLI)
	modeServe  = string(mode.ModeServe)
	modeDaemon = string(mode.ModeDaemon)
	modeDeck   = string(mode.ModeDeck)
)

func gateMode(cmd *cobra.Command) string {
	for c := cmd; c != nil; c = c.Parent() {
		if m, ok := c.Annotations[annoGateMode]; ok && m != "" {
			return m
		}
	}
	return modeCLI
}

func thinMode(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if v, ok := c.Annotations[annoThin]; ok && v != "" {
			return v == "true"
		}
	}
	return false
}

func gate(cmd *cobra.Command) error {
	if skipsOnboarding(cmd) {
		return nil
	}
	m := mode.Mode(gateMode(cmd))
	policy := mode.PolicyWarn
	if onboard.AllOrNothingAuth == "true" {
		policy = mode.PolicyBlock
	}
	err := mode.Gate(cmd.Context(), m, mode.GateHooks{
		Classify:           classifyAuth,
		CLIUnauthenticated: func(ctx context.Context) error { return cliGuidedAuth(cmd) },
		CLIUnauthorized: func(ctx context.Context) error {
			if onboard.AllOrNothingAuth == "true" {
				return errs.New(errs.KindOnboarding, "mino is not fully authorized").WithHint("%s", onboardHint())
			}
			gateWarn(cmd, onboardHint())
			return nil
		},
		UnauthorizedPolicy: policy,
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
	if m == mode.ModeServe && !thinMode(cmd) && serveSocketTaken() {
		gateWarn(cmd, "serve: a mino daemon is already listening; this instance will not own the socket")
	}
	return nil
}

func classifyAuth(ctx context.Context) mode.AuthState {
	if onboard.IsOnboarded() {
		return mode.AuthAuthorized
	}
	if !shared.GitAuthed() {
		return mode.AuthUnauthenticated
	}
	prov, id, err := shared.GitAuth()
	if err != nil {
		return mode.AuthUnauthenticated
	}
	if onboard.Check(ctx, prov, id).Ready() {
		return mode.AuthAuthorized
	}
	return mode.AuthUnauthorized
}

var (
	guidedLoginCLI = loginflow.RunCLI
	guidedOnboard  = runOnboardTo
)

func cliGuidedAuth(cmd *cobra.Command) error {
	if _, id, err := shared.GitAuth(); err != nil {
		return err
	} else if id != nil && id.ServiceIdentity() {
		return errs.New(errs.KindAuth, "git service auth is configured but not usable").
			WithHint("mino will not prompt for a personal login while %s is configured; fix that "+
				"credential, or unset it to log in as yourself", id.Origin())
	}
	p, ok := loginflow.Resolve("github")
	if !ok {
		return errs.New(errs.KindInternal, "github login provider unavailable")
	}
	status := cmd.ErrOrStderr()
	if err := guidedLoginCLI(cmd.Context(), shared, Scope(), p, cmd.InOrStdin(), status, status); err != nil {
		return err
	}
	shared.ResetGitAuth()
	return guidedOnboard(cmd, status, false)
}

func gateWarn(cmd *cobra.Command, msg string) {
	fmt.Fprintf(cmd.ErrOrStderr(), "%s %s\n", Scope().Glyphs.Warn(), msg)
}
