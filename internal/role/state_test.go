package role

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTakeLegacyActiveReadsAndRemovesTheMarker(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".data")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, activeRoleFile)
	if err := os.WriteFile(path, []byte("triage\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, ok := TakeLegacyActive(home)
	if !ok || got != "triage" {
		t.Fatalf("TakeLegacyActive = %q, %v; want triage, true", got, ok)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("marker still present, err=%v", err)
	}
	if _, ok := TakeLegacyActive(home); ok {
		t.Fatal("a second take should report no marker")
	}
}

func TestTakeLegacyActiveWithoutAMarker(t *testing.T) {
	if _, ok := TakeLegacyActive(t.TempDir()); ok {
		t.Fatal("no marker should report false")
	}
	if _, ok := TakeLegacyActive(""); ok {
		t.Fatal("empty home should report false")
	}
}
