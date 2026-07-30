package cmd

import (
	"io"
	"os"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/app/onboard"
	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/errs"
	gh "github.com/codyconfer/munin/internal/signals/github"
)

const annoSkipOnboarding = "munin_skip_onboarding"

func isCompletion(cmd *cobra.Command) bool {
	switch cmd.Name() {
	case "completion", "__complete", "__completeNoDesc":
		return true
	}
	return false
}

func skipsOnboarding(cmd *cobra.Command) bool {
	if !cmd.Runnable() {
		return true
	}
	if isCompletion(cmd) || cmd.Name() == "help" {
		return true
	}
	for c := cmd; c != nil; c = c.Parent() {
		if c.Annotations[annoSkipOnboarding] == "true" {
			return true
		}
	}
	return false
}

func enforceOnboarding(cmd *cobra.Command) error {
	if skipsOnboarding(cmd) {
		return nil
	}
	if onboard.IsOnboarded() {
		return nil
	}
	return errs.New(errs.KindOnboarding, "munin is not onboarded yet").WithHint("%s", onboard.Hint())
}

func requireOnboarding(cmd *cobra.Command) error {
	err := enforceOnboarding(cmd)
	if err != nil && errs.KindOf(err) == errs.KindOnboarding && term.IsTerminal(os.Stdout.Fd()) {
		return runOnboardTo(cmd, cmd.ErrOrStderr(), false)
	}
	return err
}

func onboardHint() string { return onboard.Hint() }

func newOnboardCmd() *cobra.Command {
	var reset, statusOnly bool
	c := &cobra.Command{
		Use:   "onboard",
		Short: "Guided setup: GitHub auth + a GitHub-verified GPG signing key",
		Long: "Walks through the checks munin requires before it will run: GitHub\n" +
			"authentication and a GPG signing key that git uses and GitHub has verified.\n" +
			"munin only inspects your setup and prints the commands to fix any gaps.",
		Args:         cobra.NoArgs,
		Annotations:  map[string]string{annoSkipOnboarding: "true"},
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if reset {
				return onboard.Reset(cmd.OutOrStdout())
			}
			return runOnboard(cmd, statusOnly)
		},
	}
	c.Flags().BoolVar(&reset, "reset", false, "clear the onboarded marker so the next run re-checks")
	c.Flags().BoolVar(&statusOnly, "status", false, "print onboarding status without prompting or saving")
	return c
}

func runOnboard(cmd *cobra.Command, statusOnly bool) error {
	return runOnboardTo(cmd, cmd.OutOrStdout(), statusOnly)
}

func runOnboardTo(cmd *cobra.Command, w io.Writer, statusOnly bool) error {
	apiURL, err := gh.NormalizeAPIURL(shared.Cfg.GitHub.APIURL)
	if err != nil {
		return err
	}
	return onboard.RunCLI(cmd.Context(), shared.Tokens, apiURL, w, statusOnly,
		func(title, message, yes, no string) (bool, error) {
			return deck.Confirm(title, message, yes, no)
		})
}
