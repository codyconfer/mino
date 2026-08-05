package loginflow

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/codyconfer/viewkit/ui"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/config"
)

func TestRunCLIKeepsCredentialPromptsOffStdout(t *testing.T) {
	orig := stdinIsTTY
	stdinIsTTY = func() bool { return true }
	t.Cleanup(func() { stdinIsTTY = orig })

	cfg := config.Defaults()
	cfg.Home = t.TempDir()
	a := &app.App{Cfg: cfg}
	t.Cleanup(a.CloseDBs)

	var loginWriter io.Writer
	p := Provider{
		Key:   "github",
		Label: "GitHub",
		Fields: []CredField{
			{Key: "github.oauth_client_id", Label: "OAuth client id", Cur: func(*app.App) string { return "" }},
		},
		Authed: func(*app.App) bool { return false },
		Login: func(_ context.Context, _ *app.App, creds map[string]string, w io.Writer) error {
			loginWriter = w
			if creds["github.oauth_client_id"] != "Iv1.typed" {
				t.Errorf("creds = %#v, want the value read from the reader", creds)
			}
			return nil
		},
	}

	var out, errOut bytes.Buffer
	if err := RunCLI(context.Background(), a, ui.Default(), p, strings.NewReader("Iv1.typed\n"), &out, &errOut); err != nil {
		t.Fatalf("RunCLI = %v", err)
	}
	if loginWriter != io.Writer(&errOut) {
		t.Error("Login should receive the error writer")
	}
	for _, unwanted := range []string{"needs OAuth client credentials", "OAuth client id"} {
		if strings.Contains(out.String(), unwanted) {
			t.Errorf("prompt %q went to stdout (would land in a redirected results file); stdout = %q", unwanted, out.String())
		}
	}
	for _, want := range []string{"needs OAuth client credentials", "OAuth client id"} {
		if !strings.Contains(errOut.String(), want) {
			t.Errorf("stderr = %q, want it to carry %q", errOut.String(), want)
		}
	}
}
