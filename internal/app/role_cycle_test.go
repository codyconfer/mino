package app

import (
	"testing"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/role"
)

func TestNextRoleOrderAndEdges(t *testing.T) {
	names := []string{"ops", "triage", "weekly"}

	got, ok := NextRole(names, "ops", 1)
	if !ok || got != "triage" {
		t.Fatalf("next from ops = %q,%v", got, ok)
	}
	got, ok = NextRole(names, "weekly", 1)
	if !ok || got != "ops" {
		t.Fatalf("wrap next = %q,%v", got, ok)
	}
	got, ok = NextRole(names, "ops", -1)
	if !ok || got != "weekly" {
		t.Fatalf("wrap prev = %q,%v", got, ok)
	}

	got, ok = NextRole(names, "", 1)
	if !ok || got != "ops" {
		t.Fatalf("empty current next = %q,%v", got, ok)
	}
	got, ok = NextRole(names, "ghost", -1)
	if !ok || got != "weekly" {
		t.Fatalf("unknown current prev = %q,%v", got, ok)
	}

	if _, ok := NextRole(nil, "ops", 1); ok {
		t.Fatal("empty names should no-op")
	}
	if _, ok := NextRole([]string{"solo"}, "solo", 1); ok {
		t.Fatal("single role wrap to self should no-op")
	}
	got, ok = NextRole([]string{"solo"}, "", 1)
	if !ok || got != "solo" {
		t.Fatalf("single role from empty = %q,%v", got, ok)
	}
}

func TestRoleCycleDebounceSkipsIntermediateHooks(t *testing.T) {
	home := t.TempDir()
	t.Cleanup(role.ClearStatusChips)

	var calls []string
	orig := role.Run
	role.Run = func(_, script string) error {
		calls = append(calls, script)
		return nil
	}
	t.Cleanup(func() { role.Run = orig })

	origCap := role.Capture
	role.Capture = func(_, script string) (string, error) { return script, nil }
	t.Cleanup(func() { role.Capture = origCap })

	dirs := &config.Directives{
		Roles: map[string]config.RoleDef{
			"ops": {
				Name: "ops",
				Hooks: config.RoleHooks{
					Enter: config.RoleShellHooks{Bash: "enter-ops", PowerShell: "enter-ops"},
					Exit:  config.RoleShellHooks{Bash: "exit-ops", PowerShell: "exit-ops"},
				},
				Status: []config.RoleStatusBlock{
					{Glyph: "slack", Bash: "ops-chip", PowerShell: "ops-chip"},
				},
			},
			"triage": {
				Name: "triage",
				Hooks: config.RoleHooks{
					Enter: config.RoleShellHooks{Bash: "enter-triage", PowerShell: "enter-triage"},
					Exit:  config.RoleShellHooks{Bash: "exit-triage", PowerShell: "exit-triage"},
				},
				Status: []config.RoleStatusBlock{
					{Glyph: "github", Bash: "triage-chip", PowerShell: "triage-chip"},
				},
			},
			"weekly": {
				Name: "weekly",
				Hooks: config.RoleHooks{
					Enter: config.RoleShellHooks{Bash: "enter-weekly", PowerShell: "enter-weekly"},
					Exit:  config.RoleShellHooks{Bash: "exit-weekly", PowerShell: "exit-weekly"},
				},
			},
		},
	}

	a := &App{Cfg: &config.Config{Home: home, Role: "ops"}, Directives: dirs}
	a.syncRoleLifecycle()
	if len(calls) != 1 || calls[0] != "enter-ops" {
		t.Fatalf("initial enter = %v", calls)
	}
	calls = nil

	genB, changed := a.BeginRoleCycle("triage")
	if !changed || a.Cfg.Role != "triage" {
		t.Fatalf("begin triage: role=%q changed=%v", a.Cfg.Role, changed)
	}
	genC, changed := a.BeginRoleCycle("weekly")
	if !changed || a.Cfg.Role != "weekly" {
		t.Fatalf("begin weekly: role=%q changed=%v", a.Cfg.Role, changed)
	}
	if len(calls) != 0 {
		t.Fatalf("hooks during burst = %v", calls)
	}
	// Intermediate role should not have replaced status chips yet.
	chips := role.StatusChips()
	if len(chips) != 1 || chips[0].Text != "ops-chip" {
		t.Fatalf("chips during burst = %+v", chips)
	}

	if a.SettleRoleCycle(genB) {
		t.Fatal("stale settle for triage should be ignored")
	}
	if len(calls) != 0 {
		t.Fatalf("hooks after stale settle = %v", calls)
	}

	if !a.SettleRoleCycle(genC) {
		t.Fatal("final settle should run")
	}
	if got := role.LoadActive(home); got != "weekly" {
		t.Fatalf("active after settle = %q", got)
	}
	if len(calls) != 2 || calls[0] != "exit-ops" || calls[1] != "enter-weekly" {
		t.Fatalf("settle hooks = %v, want exit-ops then enter-weekly", calls)
	}
}

func TestFlushRoleLifecycleAppliesPending(t *testing.T) {
	home := t.TempDir()
	var calls []string
	orig := role.Run
	role.Run = func(_, script string) error {
		calls = append(calls, script)
		return nil
	}
	t.Cleanup(func() { role.Run = orig })

	dirs := &config.Directives{
		Roles: map[string]config.RoleDef{
			"ops": {Name: "ops", Hooks: config.RoleHooks{
				Enter: config.RoleShellHooks{Bash: "enter-ops", PowerShell: "enter-ops"},
				Exit:  config.RoleShellHooks{Bash: "exit-ops", PowerShell: "exit-ops"},
			}},
			"triage": {Name: "triage", Hooks: config.RoleHooks{
				Enter: config.RoleShellHooks{Bash: "enter-triage", PowerShell: "enter-triage"},
			}},
		},
	}
	a := &App{Cfg: &config.Config{Home: home, Role: "ops"}, Directives: dirs}
	a.syncRoleLifecycle()
	calls = nil

	gen, _ := a.BeginRoleCycle("triage")
	a.FlushRoleLifecycle()
	if len(calls) != 2 || calls[0] != "exit-ops" || calls[1] != "enter-triage" {
		t.Fatalf("flush hooks = %v", calls)
	}
	if a.SettleRoleCycle(gen) {
		t.Fatal("settle after flush must be ignored")
	}
}

func TestActivateRoleCancelsPendingCycle(t *testing.T) {
	home := t.TempDir()
	var calls []string
	orig := role.Run
	role.Run = func(_, script string) error {
		calls = append(calls, script)
		return nil
	}
	t.Cleanup(func() { role.Run = orig })

	dirs := &config.Directives{
		Roles: map[string]config.RoleDef{
			"ops": {Name: "ops", Hooks: config.RoleHooks{
				Enter: config.RoleShellHooks{Bash: "enter-ops", PowerShell: "enter-ops"},
				Exit:  config.RoleShellHooks{Bash: "exit-ops", PowerShell: "exit-ops"},
			}},
			"triage": {Name: "triage", Hooks: config.RoleHooks{
				Enter: config.RoleShellHooks{Bash: "enter-triage", PowerShell: "enter-triage"},
			}},
			"weekly": {Name: "weekly", Hooks: config.RoleHooks{
				Enter: config.RoleShellHooks{Bash: "enter-weekly", PowerShell: "enter-weekly"},
			}},
		},
	}
	a := &App{Cfg: &config.Config{Home: home, Role: "ops"}, Directives: dirs}
	a.syncRoleLifecycle()
	calls = nil

	gen, _ := a.BeginRoleCycle("triage")
	if err := a.ActivateRole("weekly"); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0] != "exit-ops" || calls[1] != "enter-weekly" {
		t.Fatalf("activate hooks = %v", calls)
	}
	if a.SettleRoleCycle(gen) {
		t.Fatal("debounced settle after ActivateRole must be ignored")
	}
}
