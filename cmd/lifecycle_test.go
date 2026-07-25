package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/testenv"
)

func TestInstallAfterNukeDefaultsToStockHome(t *testing.T) {
	userHome := testenv.Isolate(t).Home

	custom := filepath.Join(userHome, "custom-munin")
	if err := config.SaveGlobalSettings(config.GlobalSettings{Home: custom}); err != nil {
		t.Fatal(err)
	}

	runLifecycle := func(args ...string) (string, error) {
		root := newRootCmd()
		var buf bytes.Buffer
		root.SetOut(&buf)
		root.SetErr(&buf)
		root.SetArgs(args)
		err := root.Execute()
		return buf.String(), err
	}

	if out, err := runLifecycle("install"); err != nil {
		t.Fatalf("install custom: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(custom, "config.yaml")); err != nil {
		t.Fatalf("expected config at custom home: %v", err)
	}

	if out, err := runLifecycle("nuke", "--yes"); err != nil {
		t.Fatalf("nuke: %v\n%s", err, out)
	}
	if _, err := os.Stat(custom); !os.IsNotExist(err) {
		t.Fatalf("nuke should remove home, stat=%v", err)
	}
	if gs := config.LoadGlobalSettings(); gs.Home != "" {
		t.Fatalf("nuke should clear settings home, got %q", gs.Home)
	}

	out, err := runLifecycle("install")
	if err != nil {
		t.Fatalf("install after nuke: %v\n%s", err, out)
	}
	want, err := config.DefaultHome()
	if err != nil {
		t.Fatal(err)
	}
	if want != filepath.Join(userHome, config.HomeDirName) {
		t.Fatalf("DefaultHome = %q, want ~/.munin under test HOME", want)
	}
	if _, err := os.Stat(filepath.Join(want, "config.yaml")); err != nil {
		t.Fatalf("install after nuke should create %s/config.yaml: %v", want, err)
	}
	if !strings.Contains(out, want) {
		t.Fatalf("install output should mention stock home %q\n%s", want, out)
	}
}

func TestInstallRespectsHomeAndDirFlags(t *testing.T) {
	userHome := testenv.Isolate(t).Home

	run := func(args ...string) error {
		root := newRootCmd()
		var buf bytes.Buffer
		root.SetOut(&buf)
		root.SetErr(&buf)
		root.SetArgs(args)
		return root.Execute()
	}

	viaHome := filepath.Join(userHome, "via-home")
	if err := run("install", "--home", viaHome); err != nil {
		t.Fatalf("--home: %v", err)
	}
	if _, err := os.Stat(filepath.Join(viaHome, "config.yaml")); err != nil {
		t.Fatalf("--home install: %v", err)
	}

	viaDir := filepath.Join(userHome, "via-dir")
	if err := run("install", "--dir", viaDir); err != nil {
		t.Fatalf("--dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(viaDir, "config.yaml")); err != nil {
		t.Fatalf("--dir install: %v", err)
	}
}
