package role

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSaveActive(t *testing.T) {
	home := t.TempDir()
	if got := LoadActive(home); got != "" {
		t.Fatalf("empty home active = %q", got)
	}
	if err := SaveActive(home, "triage"); err != nil {
		t.Fatal(err)
	}
	if got := LoadActive(home); got != "triage" {
		t.Fatalf("LoadActive = %q", got)
	}
	path := filepath.Join(home, ".data", activeRoleFile)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("state file missing: %v", err)
	}
	if err := SaveActive(home, ""); err != nil {
		t.Fatal(err)
	}
	if got := LoadActive(home); got != "" {
		t.Fatalf("cleared active = %q", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected removed state file, err=%v", err)
	}
}

func TestLoadSaveActiveEmptyHome(t *testing.T) {
	if LoadActive("") != "" {
		t.Fatal("expected empty")
	}
	if err := SaveActive("", "x"); err != nil {
		t.Fatal(err)
	}
}
