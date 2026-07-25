package app

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/testenv"
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
	fi, err := os.Stat(filepath.Join(home, "config.yaml"))
	if err != nil {
		t.Fatalf("stat config.yaml: %v", err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not modelled on Windows")
	}
	if got := di.Mode().Perm(); got != 0o700 {
		t.Errorf("home dir mode = %o, want 700", got)
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

func TestNukeRemovesHomeWithoutReinstall(t *testing.T) {
	testenv.Isolate(t)
	home := filepath.Join(t.TempDir(), "munin")
	if _, err := Install(home, false); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if err := config.SaveGlobalSettings(config.GlobalSettings{Home: home}); err != nil {
		t.Fatal(err)
	}

	var buf strings.Builder
	if err := Nuke(&buf, home); err != nil {
		t.Fatalf("Nuke: %v", err)
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("home should be gone, stat=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "config.yaml")); !os.IsNotExist(err) {
		t.Fatal("nuke must not leave/reinstall config.yaml")
	}
	if gs := config.LoadGlobalSettings(); gs.Home != "" {
		t.Fatalf("settings home should be cleared, got %q", gs.Home)
	}
	if !strings.Contains(buf.String(), "munin install") {
		t.Fatalf("nuke output should point at install, got %q", buf.String())
	}
}
