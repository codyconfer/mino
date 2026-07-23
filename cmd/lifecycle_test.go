package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDoInstallPermissions(t *testing.T) {
	home := filepath.Join(t.TempDir(), "munin")
	if _, err := doInstall(home, false); err != nil {
		t.Fatalf("doInstall: %v", err)
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
