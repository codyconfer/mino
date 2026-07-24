package token

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/codyconfer/munin/internal/auth"
)

func testKey() []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i + 1)
	}
	return k
}

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := OpenWithKey(context.Background(), filepath.Join(t.TempDir(), "tokens.duckdb"),
		func(context.Context) ([]byte, error) { return testKey(), nil })
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestPutGetDelete(t *testing.T) {
	s := openTemp(t)

	if _, ok, err := s.Get(context.Background(), "github"); ok || err != nil {
		t.Fatalf("expected miss, got ok=%v err=%v", ok, err)
	}

	want := auth.Credential{AccessToken: "a1", RefreshToken: "r1", Scope: "repo", Expiry: time.Now().Add(time.Hour)}
	if err := s.Put(context.Background(), "github", want); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.Get(context.Background(), "github")
	if err != nil || !ok {
		t.Fatalf("Get after Put: ok=%v err=%v", ok, err)
	}
	if got.AccessToken != "a1" || got.RefreshToken != "r1" || got.Scope != "repo" {
		t.Fatalf("round-trip = %+v", got)
	}

	if err := s.Put(context.Background(), "github", auth.Credential{AccessToken: "a2"}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if got, _, _ := s.Get(context.Background(), "github"); got.AccessToken != "a2" {
		t.Fatalf("expected upsert to a2, got %q", got.AccessToken)
	}

	if err := s.Delete(context.Background(), "github"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Get(context.Background(), "github"); ok {
		t.Fatal("expected miss after delete")
	}
}

func TestServicesAreIndependent(t *testing.T) {
	s := openTemp(t)
	_ = s.Put(context.Background(), "github", auth.Credential{AccessToken: "gh"})
	_ = s.Put(context.Background(), "slack", auth.Credential{AccessToken: "sl"})
	if c, _, _ := s.Get(context.Background(), "github"); c.AccessToken != "gh" {
		t.Errorf("github = %q", c.AccessToken)
	}
	if c, _, _ := s.Get(context.Background(), "slack"); c.AccessToken != "sl" {
		t.Errorf("slack = %q", c.AccessToken)
	}
}

func TestNilStore(t *testing.T) {
	var s *Store
	if _, ok, err := s.Get(context.Background(), "github"); ok || err != nil {
		t.Fatalf("nil Get = ok %v err %v", ok, err)
	}
	if err := s.Put(context.Background(), "github", auth.Credential{AccessToken: "x"}); err == nil {
		t.Fatal("nil Put should error")
	}
}
