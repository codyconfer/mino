package onboard

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/x/term"

	"github.com/codyconfer/munin/internal/auth"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/render"
	"github.com/codyconfer/munin/internal/render/glyph"
)

// ConfirmFunc asks the user whether to re-check after incomplete onboarding.
type ConfirmFunc func(title, message, yes, no string) (bool, error)

// Hint returns the standard "run munin onboard" guidance string.
func Hint() string {
	if RequiredEmailDomain != "" {
		return "run `munin onboard` to finish setup (GitHub auth + a GitHub-verified GPG or SSH signing key with a verified @" + RequiredEmailDomain + " identity)"
	}
	return "run `munin onboard` to finish setup (GitHub auth + a GitHub-verified GPG or SSH signing key)"
}

// Reset clears the onboarded marker in global settings.
func Reset(w io.Writer) error {
	gs := config.LoadGlobalSettings()
	gs.Onboarded = false
	if err := config.SaveGlobalSettings(gs); err != nil {
		return err
	}
	fmt.Fprintln(w, "cleared the onboarded marker; run `munin onboard` to set up again")
	return nil
}

// RunCLI prints onboarding status and optionally marks ready / loops on confirm.
func RunCLI(ctx context.Context, tokens auth.TokenStore, apiURL string, w io.Writer, statusOnly bool, confirm ConfirmFunc) error {
	sty := render.NewReportStyles(w)
	interactive := !statusOnly && term.IsTerminal(os.Stdout.Fd())
	for {
		st := Check(ctx, tokens, apiURL)
		PrintStatus(w, sty, st)

		if st.Ready() {
			if statusOnly {
				fmt.Fprintln(w, sty.OK.Render("✓ all checks pass."))
				return nil
			}
			gs := config.LoadGlobalSettings()
			gs.Onboarded = true
			gs.OnboardedDomain = RequiredEmailDomain
			if err := config.SaveGlobalSettings(gs); err != nil {
				return err
			}
			fmt.Fprintln(w, sty.OK.Render("✓ munin is onboarded — all commands are unlocked."))
			return nil
		}

		if !interactive {
			return errs.New(errs.KindOnboarding, "onboarding incomplete").
				WithHint("resolve the steps above, then run `munin onboard` again")
		}
		if confirm == nil {
			return errs.New(errs.KindOnboarding, "onboarding incomplete")
		}
		again, err := confirm("Onboarding incomplete",
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

// PrintStatus writes the onboarding checklist.
func PrintStatus(w io.Writer, sty render.ReportStyles, st Status) {
	fmt.Fprintln(w, sty.Title.Render("Onboarding"))
	for _, r := range st.Results {
		mark := sty.OK.Render(glyph.Check())
		if !r.OK {
			mark = sty.Err.Render(glyph.Cross())
		}
		fmt.Fprintf(w, "  %s %s\n", mark, sty.Name.Render(r.Title))
		if r.Detail != "" {
			fmt.Fprintf(w, "      %s\n", sty.Dim.Render(r.Detail))
		}
		for _, f := range r.Fix {
			fmt.Fprintf(w, "      %s %s\n", sty.Fix.Render(glyph.Arrow()), f)
		}
	}
	fmt.Fprintln(w)
}

// IsOnboarded reports whether global settings mark this domain as onboarded.
func IsOnboarded() bool {
	gs := config.LoadGlobalSettings()
	return gs.Onboarded && gs.OnboardedDomain == RequiredEmailDomain
}
