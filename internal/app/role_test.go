package app

import (
	"path/filepath"
	"testing"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/role"
)

func TestSyncRoleLifecycleRunsStatusOnEnterAndClearsOnExit(t *testing.T) {
	home := t.TempDir()
	t.Cleanup(role.ClearStatusChips)

	origRun := role.Run
	role.Run = func(string, string) error { return nil }
	t.Cleanup(func() { role.Run = origRun })

	origCap := role.Capture
	role.Capture = func(kind, script string) (string, error) {
		return script + "-out", nil
	}
	t.Cleanup(func() { role.Capture = origCap })

	dirs := &config.Directives{
		Roles: map[string]config.RoleDef{
			"triage": {
				Name: "triage",
				Hooks: config.RoleHooks{
					Enter: config.RoleShellHooks{Bash: "enter-triage", PowerShell: "enter-triage"},
					Exit:  config.RoleShellHooks{Bash: "exit-triage", PowerShell: "exit-triage"},
				},
				Status: []config.RoleStatusBlock{
					{Glyph: "github", Bash: "triage", PowerShell: "triage"},
				},
			},
			"ops": {
				Name: "ops",
				Status: []config.RoleStatusBlock{
					{Glyph: "slack", Bash: "ops", PowerShell: "ops"},
				},
			},
		},
	}

	a := &App{Cfg: &config.Config{Home: home, Role: "triage"}, Directives: dirs}
	a.syncRoleLifecycle()
	chips := role.StatusChips()
	if len(chips) != 1 || chips[0].Glyph != "github" || chips[0].Text != "triage-out" {
		t.Fatalf("enter chips = %+v", chips)
	}

	if err := a.ActivateRole("ops"); err != nil {
		t.Fatal(err)
	}
	chips = role.StatusChips()
	if len(chips) != 1 || chips[0].Glyph != "slack" || chips[0].Text != "ops-out" {
		t.Fatalf("switch chips = %+v", chips)
	}

	if err := a.ActivateRole(""); err != nil {
		t.Fatal(err)
	}
	if got := role.StatusChips(); got != nil {
		t.Fatalf("clear chips = %+v", got)
	}
}

func TestSyncRoleLifecycleRefreshesStatusWhenAlreadyActive(t *testing.T) {
	home := t.TempDir()
	t.Cleanup(role.ClearStatusChips)
	_ = role.SaveActive(home, "triage")

	origRun := role.Run
	role.Run = func(string, string) error {
		t.Fatal("enter hooks should not re-run when already active")
		return nil
	}
	t.Cleanup(func() { role.Run = origRun })

	origCap := role.Capture
	role.Capture = func(string, string) (string, error) { return "refreshed", nil }
	t.Cleanup(func() { role.Capture = origCap })

	a := &App{
		Cfg: &config.Config{Home: home, Role: "triage"},
		Directives: &config.Directives{
			Roles: map[string]config.RoleDef{
				"triage": {
					Name: "triage",
					Hooks: config.RoleHooks{
						Enter: config.RoleShellHooks{Bash: "enter", PowerShell: "enter"},
					},
					Status: []config.RoleStatusBlock{
						{Glyph: "github", Bash: "cmd", PowerShell: "cmd"},
					},
				},
			},
		},
	}
	a.syncRoleLifecycle()
	chips := role.StatusChips()
	if len(chips) != 1 || chips[0].Text != "refreshed" {
		t.Fatalf("refresh chips = %+v", chips)
	}
}

func TestSyncRoleLifecycleOrderAndPersist(t *testing.T) {
	home := t.TempDir()
	var calls []string
	orig := role.Run
	role.Run = func(kind, script string) error {
		calls = append(calls, script)
		return nil
	}
	t.Cleanup(func() { role.Run = orig })

	dirs := &config.Directives{
		Roles: map[string]config.RoleDef{
			"triage": {
				Name: "triage",
				Hooks: config.RoleHooks{
					Enter: config.RoleShellHooks{Bash: "enter-triage", PowerShell: "enter-triage"},
					Exit:  config.RoleShellHooks{Bash: "exit-triage", PowerShell: "exit-triage"},
				},
			},
			"ops": {
				Name: "ops",
				Hooks: config.RoleHooks{
					Enter: config.RoleShellHooks{Bash: "enter-ops", PowerShell: "enter-ops"},
					Exit:  config.RoleShellHooks{Bash: "exit-ops", PowerShell: "exit-ops"},
				},
			},
		},
	}

	a := &App{Cfg: &config.Config{Home: home, Role: "triage"}, Directives: dirs}
	a.syncRoleLifecycle()
	if got := role.LoadActive(home); got != "triage" {
		t.Fatalf("active after enter = %q", got)
	}
	if len(calls) != 1 || calls[0] != "enter-triage" {
		t.Fatalf("first enter calls = %v", calls)
	}

	calls = nil
	a.syncRoleLifecycle()
	if len(calls) != 0 {
		t.Fatalf("same-role hooks = %v", calls)
	}

	calls = nil
	if err := a.ActivateRole("ops"); err != nil {
		t.Fatal(err)
	}
	if a.Cfg.Role != "ops" {
		t.Fatalf("role = %q", a.Cfg.Role)
	}
	if got := role.LoadActive(home); got != "ops" {
		t.Fatalf("active after switch = %q", got)
	}
	if len(calls) != 2 || calls[0] != "exit-triage" || calls[1] != "enter-ops" {
		t.Fatalf("switch calls = %v", calls)
	}

	calls = nil
	if err := a.ActivateRole(""); err != nil {
		t.Fatal(err)
	}
	if got := role.LoadActive(home); got != "" {
		t.Fatalf("active after clear = %q", got)
	}
	if len(calls) != 1 || calls[0] != "exit-ops" {
		t.Fatalf("clear calls = %v", calls)
	}

	if _, err := filepath.Glob(filepath.Join(home, ".data", "*")); err != nil {
		t.Fatal(err)
	}
}

func TestActivateRoleSameIsNoop(t *testing.T) {
	home := t.TempDir()
	orig := role.Run
	role.Run = func(string, string) error {
		t.Fatal("should not run")
		return nil
	}
	t.Cleanup(func() { role.Run = orig })

	a := &App{
		Cfg: &config.Config{Home: home, Role: "triage"},
		Directives: &config.Directives{
			Roles: map[string]config.RoleDef{
				"triage": {Name: "triage", Hooks: config.RoleHooks{
					Enter: config.RoleShellHooks{Bash: "enter", PowerShell: "enter"},
				}},
			},
		},
	}
	_ = role.SaveActive(home, "triage")
	if err := a.ActivateRole("triage"); err != nil {
		t.Fatal(err)
	}
}

func TestSyncRoleLifecycleWarnsMissingDef(t *testing.T) {
	home := t.TempDir()
	orig := role.Run
	called := false
	role.Run = func(string, string) error {
		called = true
		return nil
	}
	t.Cleanup(func() { role.Run = orig })

	a := &App{
		Cfg:        &config.Config{Home: home, Role: "ghost"},
		Directives: &config.Directives{Roles: map[string]config.RoleDef{}},
	}
	a.syncRoleLifecycle()
	if called {
		t.Fatal("hooks should not run for undefined role")
	}
	if got := role.LoadActive(home); got != "ghost" {
		t.Fatalf("still persist intended role = %q", got)
	}
}
