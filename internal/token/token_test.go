package token

import (
	"encoding/json"
	"path/filepath"
	"strings"
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
	s, err := Open(filepath.Join(t.TempDir(), "tokens.duckdb"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	s.keyProvider = func() ([]byte, error) { return testKey(), nil }
	t.Cleanup(func() { s.Close() })
	return s
}

func TestPutGetDelete(t *testing.T) {
	s := openTemp(t)

	if _, ok, err := s.Get("github"); ok || err != nil {
		t.Fatalf("expected miss, got ok=%v err=%v", ok, err)
	}

	want := auth.Credential{AccessToken: "a1", RefreshToken: "r1", Scope: "repo", Expiry: time.Now().Add(time.Hour)}
	if err := s.Put("github", want); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.Get("github")
	if err != nil || !ok {
		t.Fatalf("Get after Put: ok=%v err=%v", ok, err)
	}
	if got.AccessToken != "a1" || got.RefreshToken != "r1" || got.Scope != "repo" {
		t.Fatalf("round-trip = %+v", got)
	}

	if err := s.Put("github", auth.Credential{AccessToken: "a2"}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if got, _, _ := s.Get("github"); got.AccessToken != "a2" {
		t.Fatalf("expected upsert to a2, got %q", got.AccessToken)
	}

	if err := s.Delete("github"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Get("github"); ok {
		t.Fatal("expected miss after delete")
	}
}

func TestServicesAreIndependent(t *testing.T) {
	s := openTemp(t)
	_ = s.Put("github", auth.Credential{AccessToken: "gh"})
	_ = s.Put("slack", auth.Credential{AccessToken: "sl"})
	if c, _, _ := s.Get("github"); c.AccessToken != "gh" {
		t.Errorf("github = %q", c.AccessToken)
	}
	if c, _, _ := s.Get("slack"); c.AccessToken != "sl" {
		t.Errorf("slack = %q", c.AccessToken)
	}
}

func TestStoredValueIsEncrypted(t *testing.T) {
	s := openTemp(t)
	if err := s.Put("github", auth.Credential{AccessToken: "super-secret-access"}); err != nil {
		t.Fatal(err)
	}
	e, ok, err := s.kv.Get(namespace, "github")
	if err != nil || !ok {
		t.Fatalf("raw kv Get: ok=%v err=%v", ok, err)
	}
	if strings.Contains(e.Value, "super-secret-access") {
		t.Fatalf("stored value leaks plaintext: %q", e.Value)
	}
}

func TestLegacyPlaintextMigration(t *testing.T) {
	s := openTemp(t)
	legacy := auth.Credential{AccessToken: "legacy-a", RefreshToken: "legacy-r", Scope: "repo"}
	b, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.kv.Put(namespace, "github", string(b), legacy.Expiry); err != nil {
		t.Fatal(err)
	}

	got, ok, err := s.Get("github")
	if err != nil || !ok {
		t.Fatalf("Get legacy: ok=%v err=%v", ok, err)
	}
	if got.AccessToken != "legacy-a" || got.RefreshToken != "legacy-r" {
		t.Fatalf("legacy read = %+v", got)
	}

	if err := s.Put("github", auth.Credential{AccessToken: "new-a"}); err != nil {
		t.Fatal(err)
	}
	e, _, _ := s.kv.Get(namespace, "github")
	if strings.Contains(e.Value, "new-a") {
		t.Fatalf("re-encrypted value leaks plaintext: %q", e.Value)
	}
	if got, ok, _ := s.Get("github"); !ok || got.AccessToken != "new-a" {
		t.Fatalf("read after re-encrypt: ok=%v got=%+v", ok, got)
	}
}

func TestKeyProviderFailureFailsClosed(t *testing.T) {
	s := openTemp(t)
	if err := s.kv.Put(namespace, "github", "whatever", time.Time{}); err != nil {
		t.Fatal(err)
	}
	s.key = nil
	s.keyProvider = func() ([]byte, error) { return nil, errUnavailable }
	if err := s.Put("github", auth.Credential{AccessToken: "x"}); err == nil {
		t.Fatal("Put should fail when key acquisition fails")
	}
	if _, _, err := s.Get("github"); err == nil {
		t.Fatal("Get should fail when key acquisition fails")
	}
}

func TestNilStore(t *testing.T) {
	var s *Store
	if _, ok, err := s.Get("x"); ok || err != nil {
		t.Errorf("nil Get = ok %v err %v", ok, err)
	}
	if err := s.Put("x", auth.Credential{}); err == nil {
		t.Error("nil Put should error")
	}
}
