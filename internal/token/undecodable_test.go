package token

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/sisyphus/sealed"

	"github.com/codyconfer/munin/internal/auth"
	"github.com/codyconfer/munin/internal/errs"
)

func otherKey() []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(200 - i)
	}
	return k
}

func openAt(t *testing.T, path string, key []byte) *Store {
	t.Helper()
	s, err := OpenWithKey(context.Background(), path, func(context.Context) ([]byte, error) { return key, nil })
	if err != nil {
		t.Fatalf("OpenWithKey: %v", err)
	}
	return s
}

func TestGetReportsUndecodableStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.duckdb")

	s := openAt(t, path, testKey())
	if err := s.Put(context.Background(), "github", auth.Credential{AccessToken: "a1", Scope: "repo"}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	lost := openAt(t, path, otherKey())
	t.Cleanup(func() { lost.Close() })

	c, ok, err := lost.Get(context.Background(), "github")
	if err == nil {
		t.Fatal("Get with the wrong key returned no error: a lost key is indistinguishable from a missing credential")
	}
	if ok {
		t.Fatalf("Get reported ok with the wrong key: %+v", c)
	}
	if !errors.Is(err, sealed.ErrUndecodable) {
		t.Fatalf("error does not carry sealed.ErrUndecodable: %v", err)
	}
	if got := errs.KindOf(err); got != errs.KindAuth {
		t.Errorf("kind = %v, want %v: a store that cannot be decrypted is an auth failure the user can fix by "+
			"re-logging in, not a %v the CLI reports as an internal storage fault", got, errs.KindAuth, got)
	}
	hint := errs.Hint(err)
	if hint == "" {
		t.Fatal("no hint: the only recovery is deleting tokens.duckdb and logging in again, which the user " +
			"cannot guess from the error text")
	}
	for _, want := range []string{"tokens.duckdb", "munin login github"} {
		if !strings.Contains(hint, want) {
			t.Errorf("hint = %q, want it to name %q", hint, want)
		}
	}
}

func TestReadCredentialDistinguishesThreeStates(t *testing.T) {
	auth.ClearCredentialStoreError()
	t.Cleanup(auth.ClearCredentialStoreError)

	path := filepath.Join(t.TempDir(), "tokens.duckdb")
	s := openAt(t, path, testKey())
	if err := s.Put(context.Background(), "slack", auth.Credential{
		AccessToken: "expired-token",
		Scope:       "channels:history",
		Expiry:      time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := s.Put(context.Background(), "google", auth.Credential{
		AccessToken: "live-token",
		Expiry:      time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if _, state, err := auth.ReadCredential(s, "github"); state != auth.CredMissing || err != nil {
		t.Fatalf("missing credential = state %v err %v, want CredMissing/nil", state, err)
	}
	c, state, err := auth.ReadCredential(s, "slack")
	if state != auth.CredExpired || err != nil {
		t.Fatalf("expired credential = state %v err %v, want CredExpired/nil", state, err)
	}
	if c.AccessToken != "expired-token" || c.Scope != "channels:history" {
		t.Fatalf("expired credential lost its payload: %+v", c)
	}
	if _, state, err := auth.ReadCredential(s, "google"); state != auth.CredValid || err != nil {
		t.Fatalf("live credential = state %v err %v, want CredValid/nil", state, err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	lost := openAt(t, path, otherKey())
	t.Cleanup(func() { lost.Close() })

	got, state, err := auth.ReadCredential(lost, "slack")
	if state != auth.CredUnreadable {
		t.Fatalf("undecryptable store = state %v (%+v), want CredUnreadable: munin reports \"not logged in\" and the user re-logs into a store it cannot read", state, got)
	}
	if err == nil || !errors.Is(err, sealed.ErrUndecodable) {
		t.Fatalf("undecryptable store error = %v, want one wrapping sealed.ErrUndecodable", err)
	}
	if stored := auth.CredentialStoreError(); stored == nil || !errors.Is(stored, sealed.ErrUndecodable) {
		t.Fatalf("CredentialStoreError() = %v, want the undecodable failure recorded for the UI", stored)
	}
	if state == auth.CredMissing {
		t.Fatal("undecryptable store collapsed into CredMissing")
	}
}
