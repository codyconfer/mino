package onboard

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/x/term"

	"github.com/codyconfer/viewkit/ui"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/gitauth"
	"github.com/codyconfer/mino/internal/render"
)

type ConfirmFunc func(title, message, yes, no string) (bool, error)

func ServiceHint(id gitauth.Identity) string {
	if id == nil || !id.ServiceIdentity() {
		return ""
	}
	if !ServiceAuthAllowed() {
		return "this mino was not built with service-auth support, so a provider-verified signing key is " +
			"still required even though " + id.Origin() + " is configured; rebuild with SERVICE_AUTH=1 " +
			"(the container image does) or authenticate as yourself"
	}
	return ""
}

func Hint(label string) string {
	if label == "" {
		label = "your git provider"
	}
	if RequiredEmailDomain != "" {
		return "run `mino onboard` to finish setup (" + label + " auth + a " + label +
			"-verified GPG or SSH signing key with a verified @" + RequiredEmailDomain + " identity)"
	}
	return "run `mino onboard` to finish setup (" + label + " auth + a " + label + "-verified GPG or SSH signing key)"
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

func RunCLI(ctx context.Context, p gitauth.Provider, id gitauth.Identity, w io.Writer, scope *ui.Scope, statusOnly bool, confirm ConfirmFunc) error {
	sty := render.NewReportStyles(w, scope)
	interactive := !statusOnly && term.IsTerminal(os.Stdout.Fd())
	for {
		st := Check(ctx, p, id)
		PrintStatus(w, sty, st)

		if st.Ready() {
			if statusOnly {
				fmt.Fprintln(w, sty.OK.Render("✓ all checks pass."))
				return nil
			}
			if ServiceAuthAllowed() && id != nil && id.ServiceIdentity() {
				fmt.Fprintln(w, sty.OK.Render("✓ running as a service identity — nothing to onboard."))
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
			e := errs.New(errs.KindOnboarding, "onboarding incomplete")
			if h := ServiceHint(id); h != "" {
				return e.WithHint("%s", h)
			}
			return e.WithHint("resolve the steps above, then run `mino onboard` again")
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
		mark := sty.OK.Render(sty.Glyphs.Check())
		if !r.OK {
			mark = sty.Err.Render(sty.Glyphs.Cross())
		}
		fmt.Fprintf(w, "  %s %s\n", mark, sty.Name.Render(r.Title))
		if r.Detail != "" {
			fmt.Fprintf(w, "      %s\n", sty.Dim.Render(r.Detail))
		}
		for _, f := range r.Fix {
			fmt.Fprintf(w, "      %s %s\n", sty.Fix.Render(sty.Glyphs.Arrow()), f)
		}
	}
	fmt.Fprintln(w)
}

func IsOnboarded() bool {
	gs := config.LoadGlobalSettings()
	return gs.Onboarded && gs.OnboardedDomain == RequiredEmailDomain
}
