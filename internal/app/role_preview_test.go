package app

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/role"
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
	orig := previewHook
	previewHook = func(_, script string) (string, error) {
		*ran = append(*ran, script)
		return "", nil
	}
	t.Cleanup(func() { previewHook = orig })
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
	orig := previewHook
	previewHook = func(_, script string) (string, error) {
		ran = append(ran, script)
		if script == "enter-triage" {
			return "", errors.New("boom")
		}
		return "", nil
	}
	t.Cleanup(func() { previewHook = orig })

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

func captureProcessStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	fn()
	os.Stdout = orig
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out
}

func TestPreviewRoleKeepsHookOutputOffTheTerminal(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not on PATH")
	}
	t.Cleanup(role.ClearStatusChips)

	a := &App{
		Cfg: &config.Config{Home: t.TempDir(), Role: "daily"},
		Directives: &config.Directives{Roles: map[string]config.RoleDef{
			"daily": {Name: "daily", Hooks: config.RoleHooks{
				Enter: config.RoleShellHooks{Bash: "echo restore-tty-leak"},
			}},
		}},
	}
	rd := config.RoleDef{Name: "loud", Hooks: config.RoleHooks{
		Enter: config.RoleShellHooks{Bash: "echo enter-tty-leak"},
		Exit:  config.RoleShellHooks{Bash: "echo exit-tty-leak"},
	}}

	var steps []RolePreviewStep
	leaked := captureProcessStdout(t, func() {
		steps = a.PreviewRole(rd, 0, func() RolePreviewStep { return RolePreviewStep{Label: "flight: inbox"} })
	})

	if strings.Contains(leaked, "tty-leak") {
		t.Fatalf("role dry-run hooks wrote to the process terminal while bubbletea owns it: %q", leaked)
	}
	details := make([]string, 0, len(steps))
	for _, s := range steps {
		details = append(details, s.Detail)
	}
	joined := strings.Join(details, "|")
	for _, want := range []string{"enter-tty-leak", "exit-tty-leak", "restore-tty-leak"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("hook output %q not captured into the dry-run report: %v", want, details)
		}
	}
}

func TestPreviewHookDoesNotInheritStdin(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not on PATH")
	}
	out, err := capturedHook("bash", "if [ -t 0 ]; then echo stdin-is-a-tty; fi; cat; echo done")
	if err != nil {
		t.Fatalf("capturedHook: %v", err)
	}
	if strings.Contains(out, "stdin-is-a-tty") {
		t.Fatalf("hook inherited the terminal on stdin: %q", out)
	}
	if !strings.Contains(out, "done") {
		t.Fatalf("hook did not run to completion (stdin never closed?): %q", out)
	}
}
