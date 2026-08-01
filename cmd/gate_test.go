package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/codyconfer/viewkit/ui"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/app/loginflow"
	"github.com/codyconfer/mino/internal/config"
)

func TestGuidedAuthKeepsPromptsAndStatusOffStdout(t *testing.T) {
	origShared := shared
	shared = &app.App{Cfg: &config.Config{Home: t.TempDir()}, Directives: &config.Directives{}}
	t.Cleanup(func() { shared = origShared })

	origLogin, origOnboard := guidedLoginCLI, guidedOnboard
	t.Cleanup(func() { guidedLoginCLI, guidedOnboard = origLogin, origOnboard })

	guidedLoginCLI = func(_ context.Context, _ *app.App, _ *ui.Scope, _ loginflow.Provider, _ io.Reader, out, errOut io.Writer) error {
		fmt.Fprintf(out, "  OAuth client id: ")
		fmt.Fprintln(errOut, "GitHub authorized — token cached.")
		return nil
	}
	guidedOnboard = func(_ *cobra.Command, w io.Writer, _ bool) error {
		fmt.Fprintln(w, "Onboarding")
		return nil
	}

	var stdout, stderr bytes.Buffer
	c := &cobra.Command{Use: "fly"}
	c.SetOut(&stdout)
	c.SetErr(&stderr)
	c.SetIn(strings.NewReader(""))

	if err := cliGuidedAuth(c); err != nil {
		t.Fatalf("cliGuidedAuth = %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("guided auth wrote to stdout (would corrupt `mino fly -o json > f.json`): %q", stdout.String())
	}
	for _, want := range []string{"OAuth client id", "token cached", "Onboarding"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr = %q, want it to carry %q", stderr.String(), want)
		}
	}
}
