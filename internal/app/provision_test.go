package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codyconfer/munin/internal/errs"
)

func TestInstallPermissions(t *testing.T) {
	home := filepath.Join(t.TempDir(), "munin")
	if _, err := Install(home, false); err != nil {
		t.Fatalf("Install: %v", err)
	}

	di, err := os.Stat(home)
	if err != nil {
		t.Fatalf("stat home: %v", err)
	}
	if got := di.Mode().Perm(); got != 0o700 {
		t.Errorf("home dir mode = %o, want 700", got)
	}

	fi, err := os.Stat(filepath.Join(home, "config.yaml"))
	if err != nil {
		t.Fatalf("stat config.yaml: %v", err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Errorf("config.yaml mode = %o, want 600", got)
	}
}

func TestInstallExistingConfigHint(t *testing.T) {
	home := filepath.Join(t.TempDir(), "munin")
	if _, err := Install(home, false); err != nil {
		t.Fatalf("Install: %v", err)
	}
	_, err := Install(home, false)
	if err == nil {
		t.Fatal("expected already-installed error")
	}
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("want *errs.Error, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "already has a config file") {
		t.Fatalf("error = %v", err)
	}
	if e.Hint == "" {
		t.Fatal("expected reinstall hint")
	}
}

func TestInstallForceDoesNotMisreportExists(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(parent, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Install(filepath.Join(parent, "munin"), true)
	if err == nil {
		t.Fatal("expected install failure")
	}
	if strings.Contains(err.Error(), "already has a config file") {
		t.Fatalf("misreported exists: %v", err)
	}
}
