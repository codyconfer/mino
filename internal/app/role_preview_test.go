package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/role"
)

func previewDirectives() *config.Directives {
	return &config.Directives{
		Roles: map[string]config.RoleDef{
			"triage": {
				Name: "triage",
				Hooks: config.RoleHooks{
					Enter: config.RoleShellHooks{Bash: "enter-triage"},
					Exit:  config.RoleShellHooks{Bash: "exit-triage"},
				},
			},
			"daily": {
				Name:  "daily",
				Hooks: config.RoleHooks{Enter: config.RoleShellHooks{Bash: "enter-daily"}},
			},
		},
	}
}

func recordRuns(t *testing.T, ran *[]string) {
	t.Helper()
	orig := role.Run
	role.Run = func(_, script string) error {
		*ran = append(*ran, script)
		return nil
	}
	t.Cleanup(func() { role.Run = orig })
}

func TestPreviewRoleRunsHooksAroundBodyThenRestores(t *testing.T) {
	t.Cleanup(role.ClearStatusChips)
	var ran []string
	recordRuns(t, &ran)

	a := &App{Cfg: &config.Config{Home: t.TempDir(), Role: "daily"}, Directives: previewDirectives()}
	steps := a.PreviewRole(a.Directives.Roles["triage"], 0, func() RolePreviewStep {
		ran = append(ran, "body")
		return RolePreviewStep{Label: "flight: inbox", Detail: "3 items"}
	})

	want := []string{"enter-triage", "body", "exit-triage", "enter-daily"}
	if strings.Join(ran, ",") != strings.Join(want, ",") {
		t.Fatalf("run order = %v, want %v", ran, want)
	}

	labels := make([]string, 0, len(steps))
	for _, s := range steps {
		labels = append(labels, s.Label)
	}
	wantLabels := []string{"enter hook (bash)", "flight: inbox", "exit hook (bash)", "restored role: daily"}
	if strings.Join(labels, "|") != strings.Join(wantLabels, "|") {
		t.Fatalf("step labels = %v, want %v", labels, wantLabels)
	}
}

func TestPreviewRoleHoldsBetweenBodyAndExit(t *testing.T) {
	t.Cleanup(role.ClearStatusChips)
	var ran []string
	recordRuns(t, &ran)

	a := &App{Cfg: &config.Config{Home: t.TempDir()}, Directives: previewDirectives()}
	start := time.Now()
	steps := a.PreviewRole(a.Directives.Roles["triage"], 30*time.Millisecond, func() RolePreviewStep {
		return RolePreviewStep{Label: "flight: inbox"}
	})
	if elapsed := time.Since(start); elapsed < 30*time.Millisecond {
		t.Fatalf("preview returned in %v, want at least the hold", elapsed)
	}

	var held bool
	for _, s := range steps {
		if strings.HasPrefix(s.Label, "held ") {
			held = true
		}
	}
	if !held {
		t.Fatalf("no hold step recorded: %+v", steps)
	}
}

func TestPreviewRoleWithNoActiveRoleReportsNoRestore(t *testing.T) {
	t.Cleanup(role.ClearStatusChips)
	var ran []string
	recordRuns(t, &ran)

	a := &App{Cfg: &config.Config{Home: t.TempDir()}, Directives: previewDirectives()}
	steps := a.PreviewRole(a.Directives.Roles["triage"], 0, func() RolePreviewStep {
		return RolePreviewStep{Label: "flight: inbox"}
	})

	last := steps[len(steps)-1]
	if last.Label != "restored: no active role" {
		t.Fatalf("last step = %q, want the no-active-role restore", last.Label)
	}
	if strings.Join(ran, ",") != "enter-triage,exit-triage" {
		t.Fatalf("run order = %v, want only the previewed role's hooks", ran)
	}
}

func TestPreviewRoleSurfacesHookErrorsAndStillRestores(t *testing.T) {
	t.Cleanup(role.ClearStatusChips)
	var ran []string
	orig := role.Run
	role.Run = func(_, script string) error {
		ran = append(ran, script)
		if script == "enter-triage" {
			return errors.New("boom")
		}
		return nil
	}
	t.Cleanup(func() { role.Run = orig })

	a := &App{Cfg: &config.Config{Home: t.TempDir(), Role: "daily"}, Directives: previewDirectives()}
	steps := a.PreviewRole(a.Directives.Roles["triage"], 0, func() RolePreviewStep {
		ran = append(ran, "body")
		return RolePreviewStep{Label: "flight: inbox"}
	})

	if steps[0].Err == nil || !strings.Contains(steps[0].Err.Error(), "boom") {
		t.Fatalf("enter step did not carry the hook error: %+v", steps[0])
	}
	want := []string{"enter-triage", "body", "exit-triage", "enter-daily"}
	if strings.Join(ran, ",") != strings.Join(want, ",") {
		t.Fatalf("run order = %v, want %v", ran, want)
	}
}

func TestPreviewRoleWithoutHooksJustRunsTheBody(t *testing.T) {
	t.Cleanup(role.ClearStatusChips)
	var ran []string
	recordRuns(t, &ran)

	a := &App{Cfg: &config.Config{Home: t.TempDir(), Role: "daily"}, Directives: previewDirectives()}
	start := time.Now()
	steps := a.PreviewRole(config.RoleDef{Name: "bare"}, time.Hour, func() RolePreviewStep {
		return RolePreviewStep{Label: "flight: inbox"}
	})
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("hookless preview waited %v; it should not hold", elapsed)
	}
	if len(steps) != 1 || steps[0].Label != "flight: inbox" {
		t.Fatalf("steps = %+v, want just the body step", steps)
	}
	if len(ran) != 0 {
		t.Fatalf("hookless preview ran scripts: %v", ran)
	}
}

func TestPreviewRoleIsInertInThinMode(t *testing.T) {
	var ran []string
	recordRuns(t, &ran)

	a := &App{Cfg: &config.Config{Home: t.TempDir(), Role: "daily"}, Directives: previewDirectives(), thin: true}
	steps := a.PreviewRole(a.Directives.Roles["triage"], time.Hour, func() RolePreviewStep {
		return RolePreviewStep{Label: "flight: inbox"}
	})

	if len(steps) != 1 || steps[0].Label != "flight: inbox" {
		t.Fatalf("steps = %+v, want just the body step", steps)
	}
	if len(ran) != 0 {
		t.Fatalf("thin mode ran hooks: %v", ran)
	}
}
