package onboard

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/x/term"

	"github.com/codyconfer/mino/internal/auth"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/render"
	"github.com/codyconfer/mino/internal/render/glyph"
)

type ConfirmFunc func(title, message, yes, no string) (bool, error)

func Hint() string {
	if RequiredEmailDomain != "" {
		return "run `mino onboard` to finish setup (GitHub auth + a GitHub-verified GPG or SSH signing key with a verified @" + RequiredEmailDomain + " identity)"
	}
	return "run `mino onboard` to finish setup (GitHub auth + a GitHub-verified GPG or SSH signing key)"
}

func Reset(w io.Writer) error {
	gs := config.LoadGlobalSettings()
	gs.Onboarded = false
	if err := config.SaveGlobalSettings(gs); err != nil {
		return err
	}
	fmt.Fprintln(w, "cleared the onboarded marker; run `mino onboard` to set up again")
	return nil
}

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
			fmt.Fprintln(w, sty.OK.Render("✓ mino is onboarded — all commands are unlocked."))
			return nil
		}

		if !interactive {
			return errs.New(errs.KindOnboarding, "onboarding incomplete").
				WithHint("resolve the steps above, then run `mino onboard` again")
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

func IsOnboarded() bool {
	gs := config.LoadGlobalSettings()
	return gs.Onboarded && gs.OnboardedDomain == RequiredEmailDomain
}
