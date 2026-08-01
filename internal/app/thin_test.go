package app

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/codyconfer/mino/internal/config"
)

func writeThinHome(t *testing.T, marker string) string {
	t.Helper()
	home := t.TempDir()
	cfg := "role: triage\n"
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	role := "type: role\nname: triage\nflights: [default]\nqueries: []\nhooks:\n  enter:\n    bash: touch " + marker + "\n"
	if err := os.WriteFile(filepath.Join(home, "triage.yaml"), []byte(role), 0o600); err != nil {
		t.Fatal(err)
	}
	return home
}

func duckFiles(t *testing.T, home string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(config.DataDir(home), "*.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

func TestThinLoadOpensNoDatabasesAndRunsNoRoleHooks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("role enter hook uses bash")
	}
	marker := filepath.Join(t.TempDir(), "entered")
	home := writeThinHome(t, marker)

	a, err := Load(Options{Home: home, Thin: true})
	if err != nil {
		t.Fatalf("thin Load: %v", err)
	}
	defer a.Shutdown()

	if !a.Thin() {
		t.Error("Thin() should report true")
	}
	if a.Mgr != nil {
		t.Error("thin load must not open the config store")
	}
	if a.Tokens != nil {
		t.Error("thin load must not open the token store")
	}
	if a.Audit != nil {
		t.Error("thin load must not open the audit store")
	}
	if a.Cache != nil {
		t.Error("thin load must not open the cache store")
	}
	if files := duckFiles(t, home); len(files) != 0 {
		t.Errorf("thin load created DuckDB files: %v", files)
	}
	if _, err := os.Stat(filepath.Join(config.DataDir(home), "active-role")); !os.IsNotExist(err) {
		t.Error("thin load must not write .data/active-role")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Error("thin load must not run role enter hooks")
	}

	if a.Cfg.Role != "triage" {
		t.Errorf("role = %q, want triage read from files", a.Cfg.Role)
	}
	if _, ok := a.Directives.Roles["triage"]; !ok {
		t.Errorf("directives should load from files, got roles %v", a.Directives.RoleNames())
	}
}

func TestNonThinLoadOwnsDatabasesAndRunsRoleHooks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("role enter hook uses bash")
	}
	marker := filepath.Join(t.TempDir(), "entered")
	home := writeThinHome(t, marker)

	a, err := Load(Options{Home: home, Reconcile: config.ReconcileApply})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer a.Shutdown()

	if a.Thin() {
		t.Error("Thin() should report false")
	}
	if a.Cache == nil {
		t.Error("normal load should own the cache store")
	}
	if files := duckFiles(t, home); len(files) == 0 {
		t.Error("normal load should own at least one DuckDB file")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("normal load should run role enter hooks: %v", err)
	}
}
