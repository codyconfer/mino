package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/onboard"
	gh "github.com/codyconfer/munin/internal/signals/github"
	"github.com/codyconfer/munin/internal/ui"
)

const annoSkipOnboarding = "munin_skip_onboarding"

func skipsOnboarding(cmd *cobra.Command) bool {
	if !cmd.Runnable() {
		return true
	}
	switch cmd.Name() {
	case "help", "completion", "__complete", "__completeNoDesc":
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
	gs := config.LoadGlobalSettings()
	if gs.Onboarded && gs.OnboardedDomain == onboard.RequiredEmailDomain {
		return nil
	}
	return errs.New(errs.KindOnboarding, "munin is not onboarded yet").WithHint("%s", onboardHint())
}

func onboardHint() string {
	if onboard.RequiredEmailDomain != "" {
		return "run `munin onboard` to finish setup (GitHub auth + a GitHub-verified GPG signing key with a verified @" + onboard.RequiredEmailDomain + " identity)"
	}
	return "run `munin onboard` to finish setup (GitHub auth + a GitHub-verified GPG signing key)"
}

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
				return resetOnboarding(cmd)
			}
			return runOnboard(cmd, statusOnly)
		},
	}
	c.Flags().BoolVar(&reset, "reset", false, "clear the onboarded marker so the next run re-checks")
	c.Flags().BoolVar(&statusOnly, "status", false, "print onboarding status without prompting or saving")
	return c
}

func resetOnboarding(cmd *cobra.Command) error {
	gs := config.LoadGlobalSettings()
	gs.Onboarded = false
	if err := config.SaveGlobalSettings(gs); err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), "cleared the onboarded marker; run `munin onboard` to set up again")
	return nil
}

func runOnboard(cmd *cobra.Command, statusOnly bool) error {
	w := cmd.OutOrStdout()
	sty := newOnboardStyles(w)
	apiURL, err := gh.NormalizeAPIURL(shared.cfg.GitHub.APIURL)
	if err != nil {
		return err
	}

	interactive := !statusOnly && term.IsTerminal(int(os.Stdout.Fd()))
	for {
		st := onboard.Check(cmd.Context(), shared.tokens, apiURL)
		printOnboardStatus(w, sty, st)

		if st.Ready() {
			if statusOnly {
				fmt.Fprintln(w, sty.ok.Render("✓ all checks pass."))
				return nil
			}
			gs := config.LoadGlobalSettings()
			gs.Onboarded = true
			gs.OnboardedDomain = onboard.RequiredEmailDomain
			if err := config.SaveGlobalSettings(gs); err != nil {
				return err
			}
			fmt.Fprintln(w, sty.ok.Render("✓ munin is onboarded — all commands are unlocked."))
			return nil
		}

		if !interactive {
			return errs.New(errs.KindOnboarding, "onboarding incomplete").
				WithHint("resolve the steps above, then run `munin onboard` again")
		}

		again, err := ui.Confirm("Onboarding incomplete",
			"Run the commands above in another shell, then re-check. Re-check now?",
			"Re-check", "Quit")
		if err != nil {
			return err
		}
		if !again {
			return errs.New(errs.KindOnboarding, "onboarding incomplete")
		}
		fmt.Fprintln(w)
	}
}

type onboardStyles struct {
	title, ok, bad, name, dim, fix lipgloss.Style
}

func newOnboardStyles(w io.Writer) onboardStyles {
	r := lipgloss.NewRenderer(w)
	return onboardStyles{
		title: r.NewStyle().Bold(true).Underline(true),
		ok:    r.NewStyle().Foreground(lipgloss.Color("10")),
		bad:   r.NewStyle().Foreground(lipgloss.Color("9")).Bold(true),
		name:  r.NewStyle().Bold(true),
		dim:   r.NewStyle().Faint(true),
		fix:   r.NewStyle().Foreground(lipgloss.Color("12")),
	}
}

func printOnboardStatus(w io.Writer, s onboardStyles, st onboard.Status) {
	fmt.Fprintln(w, s.title.Render("Onboarding"))
	for _, r := range st.Results {
		mark := s.ok.Render("✓")
		if !r.OK {
			mark = s.bad.Render("✗")
		}
		fmt.Fprintf(w, "  %s %s\n", mark, s.name.Render(r.Title))
		if r.Detail != "" {
			fmt.Fprintf(w, "      %s\n", s.dim.Render(r.Detail))
		}
		for _, f := range r.Fix {
			fmt.Fprintf(w, "      %s %s\n", s.fix.Render("→"), f)
		}
	}
	fmt.Fprintln(w)
}
