package gcx

import (
	"context"
	"strings"
	"testing"

	"github.com/codyconfer/mino/plugin"
)

func TestNormalizeStack(t *testing.T) {
	ok := []struct{ in, want string }{
		{"myorg.grafana.net", "myorg.grafana.net"},
		{"https://myorg.grafana.net", "myorg.grafana.net"},
		{"https://myorg.grafana.net/", "myorg.grafana.net"},
		{"http://myorg.grafana.net", "myorg.grafana.net"},
		{"  HTTPS://MyOrg.Grafana.Net/  ", "myorg.grafana.net"},
	}
	for _, tc := range ok {
		got, err := normalizeStack(tc.in)
		if err != nil || got != tc.want {
			t.Fatalf("normalizeStack(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
	for _, bad := range []string{"", "   ", "ftp://myorg.grafana.net", "myorg.grafana.net/path", "my org", "https://"} {
		if got, err := normalizeStack(bad); err == nil {
			t.Fatalf("normalizeStack(%q) = %q, want error", bad, got)
		}
	}
}

func TestIRMBaseURL(t *testing.T) {
	got, err := IRMBaseURL("https://myorg.grafana.net/")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://myorg.grafana.net" + IRMAPIPath
	if got != want {
		t.Fatalf("IRMBaseURL = %q want %q", got, want)
	}
}

func TestResolveStackPrecedence(t *testing.T) {
	ctx := context.Background()
	settings := map[string]any{"stack": "from-setting.grafana.net"}

	pinStack(t, "from-context.grafana.net")

	got, err := ResolveStack(ctx, "from-param.grafana.net", settings)
	if err != nil || got != "from-param.grafana.net" {
		t.Fatalf("param rung = %q, %v", got, err)
	}

	got, err = ResolveStack(ctx, "", settings)
	if err != nil || got != "from-context.grafana.net" {
		t.Fatalf("context rung = %q, %v", got, err)
	}

	shared.cur = ""
	got, err = ResolveStack(ctx, "", settings)
	if err != nil || got != "from-setting.grafana.net" {
		t.Fatalf("setting rung = %q, %v", got, err)
	}

	if _, err = ResolveStack(ctx, "", nil); err == nil {
		t.Fatal("expected an error when no stack is bound")
	} else if !strings.Contains(hintOf(err), "plugins.gcx.stack") {
		t.Fatalf("hint = %q", hintOf(err))
	}
}

func TestTokenAndAuthed(t *testing.T) {
	clearEnv(t)
	store := memStore{}

	if _, err := Token(store, DefaultTokenEnv); err == nil {
		t.Fatal("expected an error with no token anywhere")
	} else if !strings.Contains(hintOf(err), "mino login gcx") {
		t.Fatalf("hint = %q", hintOf(err))
	}
	if Authed(store, DefaultTokenEnv) {
		t.Fatal("Authed with no token")
	}

	store[TokenKey] = plugin.Credential{AccessToken: "glsa_sealed", Scope: CredScope}
	got, err := Token(store, DefaultTokenEnv)
	if err != nil || got != "glsa_sealed" {
		t.Fatalf("sealed token = %q, %v", got, err)
	}
	if !Authed(store, DefaultTokenEnv) {
		t.Fatal("Authed false with a sealed token")
	}

	t.Setenv(DefaultTokenEnv, "glsa_env")
	got, err = Token(store, DefaultTokenEnv)
	if err != nil || got != "glsa_env" {
		t.Fatalf("env should win: %q, %v", got, err)
	}
	if !Authed(memStore{}, DefaultTokenEnv) {
		t.Fatal("Authed false with only the env override")
	}
}

func TestFromHostTokenEnvOverride(t *testing.T) {
	h := newFakeHost(map[string]any{"token_env": "MY_GCX_TOKEN"})
	cfg := FromHost(h)
	if cfg.TokenEnv != "MY_GCX_TOKEN" {
		t.Fatalf("TokenEnv = %q", cfg.TokenEnv)
	}
	if cfg.Store == nil {
		t.Fatal("expected a credential store")
	}
	if FromHost(nil).TokenEnv != DefaultTokenEnv {
		t.Fatal("nil host should still carry the default token env")
	}
}
