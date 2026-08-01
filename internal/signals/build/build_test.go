package build

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/codyconfer/mino/internal/config"
	gh "github.com/codyconfer/mino/internal/signals/github"
)

func fakeGHOnPath(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake gh shell script requires a POSIX shell")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

func TestGithubBackendPinsConfiguredHostname(t *testing.T) {
	fakeGHOnPath(t)

	cfg := &config.Config{}
	cfg.GitHub.APIURL = "https://ghe.example.com/api/v3"
	backend, err := githubBackend(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	cli, ok := backend.(gh.CLIBackend)
	if !ok {
		t.Fatalf("backend = %T, want gh.CLIBackend", backend)
	}
	if cli.Hostname != "ghe.example.com" {
		t.Fatalf("Hostname = %q, want ghe.example.com", cli.Hostname)
	}

	cfg.GitHub.APIURL = ""
	backend, err = githubBackend(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	cli, ok = backend.(gh.CLIBackend)
	if !ok {
		t.Fatalf("backend = %T, want gh.CLIBackend", backend)
	}
	if cli.Hostname != "" {
		t.Fatalf("Hostname = %q, want empty", cli.Hostname)
	}
}

func TestResolveWriteTarget(t *testing.T) {
	if _, err := ResolveWriteTarget("task list", "tasks.list", "", ""); err == nil {
		t.Error("expected error when no writable target is configured")
	}

	if got, err := ResolveWriteTarget("task list", "tasks.list", "My Tasks", ""); err != nil || got != "My Tasks" {
		t.Errorf("default target = %q, %v", got, err)
	}

	if got, err := ResolveWriteTarget("task list", "tasks.list", "My Tasks", "my tasks"); err != nil || got != "My Tasks" {
		t.Errorf("matching target = %q, %v", got, err)
	}

	if _, err := ResolveWriteTarget("directory", "drive.dir", "Inbox", "Someone Else's Folder"); err == nil {
		t.Error("expected other targets to be rejected as read-only")
	}
}
