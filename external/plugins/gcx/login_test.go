package gcx

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/codyconfer/mino/plugin"
)

func pipeStdin(t *testing.T, tty bool, content string) {
	t.Helper()
	prevTTY, prevIn := stdinIsTTY, stdin
	stdinIsTTY = func() bool { return tty }
	stdin = strings.NewReader(content)
	t.Cleanup(func() { stdinIsTTY, stdin = prevTTY, prevIn })
}

func TestLoginProviderShape(t *testing.T) {
	p := LoginProvider()
	if p.PluginID != PluginID || p.Key != TokenKey {
		t.Fatalf("provider = %#v", p)
	}
	if len(p.Signals) != 1 || p.Signals[0] != SignalName {
		t.Fatalf("signals = %v", p.Signals)
	}
	if p.Login == nil || p.Authed == nil {
		t.Fatal("provider needs both Login and Authed")
	}
}

// TestLoginProviderHasNoFields guards the cleartext-persist hazard documented in
// SPIKE.md §6: loginflow writes every prompted LoginField into config.yaml
// before Login runs.
func TestLoginProviderHasNoFields(t *testing.T) {
	if fields := LoginProvider().Fields; len(fields) != 0 {
		t.Fatalf("LoginProvider must declare no Fields, got %#v", fields)
	}
}

func TestLoginProviderAuthed(t *testing.T) {
	clearEnv(t)
	h := newFakeHost(nil)
	p := LoginProvider()
	if p.Authed(h) {
		t.Fatal("Authed with no token")
	}
	h.store[TokenKey] = plugin.Credential{AccessToken: "glsa_x"}
	if !p.Authed(h) {
		t.Fatal("Authed false with a sealed token")
	}
}

func TestReadTokenFromEnv(t *testing.T) {
	t.Setenv(DefaultTokenEnv, "  glsa_from_env  ")
	var w bytes.Buffer
	got, err := readToken(&w, DefaultTokenEnv)
	if err != nil || got != "glsa_from_env" {
		t.Fatalf("readToken = %q, %v", got, err)
	}
	if !strings.Contains(w.String(), DefaultTokenEnv) {
		t.Fatalf("expected a note naming the env var, got %q", w.String())
	}
}

func TestReadTokenFromPipedStdin(t *testing.T) {
	clearEnv(t)
	pipeStdin(t, false, "glsa_piped\n")
	got, err := readToken(io.Discard, DefaultTokenEnv)
	if err != nil || got != "glsa_piped" {
		t.Fatalf("readToken = %q, %v", got, err)
	}
}

func TestReadTokenEmptyStdin(t *testing.T) {
	clearEnv(t)
	pipeStdin(t, false, "   \n")
	_, err := readToken(io.Discard, DefaultTokenEnv)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(hintOf(err), DefaultTokenEnv) {
		t.Fatalf("hint = %q", hintOf(err))
	}
}

func TestLoginSealsToken(t *testing.T) {
	clearEnv(t)
	pipeStdin(t, false, "glsa_sealed_me\n")
	h := newFakeHost(nil)

	if err := login(context.Background(), h, nil, io.Discard); err != nil {
		t.Fatal(err)
	}
	c, ok := h.store[TokenKey]
	if !ok {
		t.Fatal("token was not sealed")
	}
	if c.AccessToken != "glsa_sealed_me" || c.Scope != CredScope {
		t.Fatalf("credential = %#v", c)
	}
}

func TestLoginWarnsOnUnexpectedPrefix(t *testing.T) {
	clearEnv(t)
	pipeStdin(t, false, "not-a-grafana-token\n")
	var w bytes.Buffer
	if err := login(context.Background(), newFakeHost(nil), nil, &w); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(w.String(), "glsa_") {
		t.Fatalf("expected a prefix warning, got %q", w.String())
	}
}

func TestLoginWithoutStore(t *testing.T) {
	clearEnv(t)
	if err := login(context.Background(), nil, nil, io.Discard); err == nil {
		t.Fatal("expected an error without a credential store")
	}
}
