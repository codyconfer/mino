package role

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/codyconfer/munin/internal/config"
)

func TestSelectPrefersPlatformShell(t *testing.T) {
	both := config.RoleShellHooks{
		Bash:       "echo bash",
		PowerShell: "Write-Host ps",
	}
	kind, script, ok := Select(both)
	if !ok {
		t.Fatal("expected selection")
	}
	if runtime.GOOS == "windows" {
		if kind != "powershell" || script != "Write-Host ps" {
			t.Fatalf("windows select = %s %q", kind, script)
		}
	} else if kind != "bash" || script != "echo bash" {
		t.Fatalf("unix select = %s %q", kind, script)
	}
}

func TestSelectFallsBackWhenPreferredEmpty(t *testing.T) {
	if runtime.GOOS == "windows" {
		kind, script, ok := Select(config.RoleShellHooks{Bash: "echo only"})
		if !ok || kind != "bash" || script != "echo only" {
			t.Fatalf("got %s %q ok=%v", kind, script, ok)
		}
		return
	}
	kind, script, ok := Select(config.RoleShellHooks{PowerShell: "Write-Host only"})
	if !ok || kind != "powershell" || script != "Write-Host only" {
		t.Fatalf("got %s %q ok=%v", kind, script, ok)
	}
}

func TestSelectEmpty(t *testing.T) {
	if _, _, ok := Select(config.RoleShellHooks{}); ok {
		t.Fatal("expected no selection")
	}
}

func TestRunHooksUsesRunner(t *testing.T) {
	var gotKind, gotScript string
	orig := Run
	Run = func(kind, script string) error {
		gotKind, gotScript = kind, script
		return nil
	}
	t.Cleanup(func() { Run = orig })

	hooks := config.RoleShellHooks{Bash: "echo hi", PowerShell: "Write-Host hi"}
	if err := RunHooks(hooks); err != nil {
		t.Fatal(err)
	}
	wantKind, wantScript, _ := Select(hooks)
	if gotKind != wantKind || gotScript != wantScript {
		t.Fatalf("ran %s %q, want %s %q", gotKind, gotScript, wantKind, wantScript)
	}
}

func TestRunHooksNoopWhenEmpty(t *testing.T) {
	orig := Run
	Run = func(string, string) error {
		t.Fatal("runner should not be called")
		return nil
	}
	t.Cleanup(func() { Run = orig })
	if err := RunHooks(config.RoleShellHooks{}); err != nil {
		t.Fatal(err)
	}
}

func TestRunEnterExit(t *testing.T) {
	var calls []string
	orig := Run
	Run = func(kind, script string) error {
		calls = append(calls, kind+":"+script)
		return nil
	}
	t.Cleanup(func() { Run = orig })

	rd := config.RoleDef{
		Hooks: config.RoleHooks{
			Enter: config.RoleShellHooks{Bash: "enter-bash", PowerShell: "enter-ps"},
			Exit:  config.RoleShellHooks{Bash: "exit-bash", PowerShell: "exit-ps"},
		},
	}
	if err := RunEnter(rd); err != nil {
		t.Fatal(err)
	}
	if err := RunExit(rd); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %v", calls)
	}
}

func TestRunHooksPropagatesError(t *testing.T) {
	orig := Run
	Run = func(string, string) error { return errors.New("boom") }
	t.Cleanup(func() { Run = orig })
	err := RunHooks(config.RoleShellHooks{Bash: "x", PowerShell: "y"})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("err = %v", err)
	}
}

func TestDefaultRunBashWritesFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash hook smoke is unix-oriented")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		if _, err2 := exec.LookPath("sh"); err2 != nil {
			t.Skip("no bash/sh on PATH")
		}
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "ok")
	script := "printf ok > " + marker
	if err := Run("bash", script); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(b)) != "ok" {
		t.Fatalf("marker = %q", b)
	}
}
