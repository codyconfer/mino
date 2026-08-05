package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/app/loginflow"
	"github.com/codyconfer/mino/internal/auth"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/testenv"
	"github.com/codyconfer/mino/internal/token"
)

func fixedKey(b byte) func(context.Context) ([]byte, error) {
	return func(context.Context) ([]byte, error) { return bytes.Repeat([]byte{b}, 32), nil }
}

func sharedWithTokenStore(t *testing.T, rekey bool) {
	t.Helper()
	testenv.Isolate(t)
	orig := shared
	t.Cleanup(func() { shared = orig })
	t.Cleanup(auth.ClearCredentialStoreError)
	auth.ClearCredentialStoreError()

	home := t.TempDir()
	path := config.DataPath(home, config.TokensDB)

	first, err := token.OpenWithKey(context.Background(), path, fixedKey(1))
	if err != nil {
		t.Skipf("token store unavailable: %v", err)
	}
	cred := auth.Credential{AccessToken: "gho_test", Expiry: time.Now().Add(time.Hour)}
	if err := first.Put(context.Background(), "github", cred); err != nil {
		t.Skipf("token store write unavailable: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	key := fixedKey(1)
	if rekey {
		key = fixedKey(2)
	}
	reopened, err := token.OpenWithKey(context.Background(), path, key)
	if err != nil {
		t.Fatalf("reopen token store: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })

	shared = &app.App{
		Cfg:        &config.Config{Home: home, Output: "terminal"},
		Directives: &config.Directives{},
		Tokens:     reopened,
	}
	closeSharedDBs(t)
}

func TestLoginRefusesAnUnreadableCredentialStore(t *testing.T) {
	sharedWithTokenStore(t, true)

	p, err := loginflow.ResolveOrErr("github")
	if err != nil {
		t.Fatal(err)
	}
	err = refuseUnreadableStore(p)
	if err == nil {
		t.Fatal("mino still lets the user re-login into a credential store it cannot read")
	}
	msg := err.Error()
	for _, want := range []string{"cannot be decrypted", config.TokensDB} {
		if !strings.Contains(msg, want) && !strings.Contains(errs.Hint(err), want) {
			t.Fatalf("refusal %q does not tell the user about %q", msg+" / "+errs.Hint(err), want)
		}
	}
}

func TestLoginProceedsWhenTheCredentialStoreIsReadable(t *testing.T) {
	sharedWithTokenStore(t, false)

	p, err := loginflow.ResolveOrErr("github")
	if err != nil {
		t.Fatal(err)
	}
	if err := refuseUnreadableStore(p); err != nil {
		t.Fatalf("refused a healthy credential store: %v", err)
	}
}

func TestLoginRefusalIsInertWithoutATokenStore(t *testing.T) {
	orig := shared
	t.Cleanup(func() { shared = orig })
	shared = &app.App{Cfg: &config.Config{Home: t.TempDir()}, Directives: &config.Directives{}}
	closeSharedDBs(t)

	p, err := loginflow.ResolveOrErr("github")
	if err != nil {
		t.Fatal(err)
	}
	if err := refuseUnreadableStore(p); err != nil {
		t.Fatalf("refused with no token store open: %v", err)
	}
}
