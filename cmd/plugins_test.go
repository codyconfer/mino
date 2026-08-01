package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/plugin"
	"github.com/codyconfer/mino/internal/signals/build"
	"github.com/codyconfer/mino/internal/testenv"
)

func TestPluginsHelpStatesCompileTimeTruth(t *testing.T) {
	root := newRootCmd()
	root.SetArgs([]string{"plugins", "--help"})
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"compile time",
		"install",
		"uninstall",
		"list",
		"enable",
		"disable",
		"scaffold",
		".so",
	} {
		if !strings.Contains(strings.ToLower(out), strings.ToLower(want)) {
			t.Errorf("plugins help missing %q\n%s", want, out)
		}
	}
}

func TestPluginsScaffoldCLI(t *testing.T) {
	testenv.Isolate(t)
	dir := t.TempDir()
	outDir := filepath.Join(dir, "widgets")
	root := newRootCmd()
	root.SetArgs([]string{"plugins", "scaffold", "acme.widgets", "--dir", outDir})
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	if err := root.Execute(); err != nil {
		t.Fatalf("scaffold: %v\n%s", err, buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "scaffolded acme.widgets") {
		t.Fatalf("output: %s", out)
	}
	body, err := os.ReadFile(filepath.Join(outDir, "plugin.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `PluginID    = "acme.widgets"`) {
		t.Fatalf("plugin.go = %s", body)
	}
	if !strings.Contains(string(body), "github.com/codyconfer/mino/plugin") {
		t.Fatal("expected public SDK import")
	}
}

func TestPluginsInstallUninstallCLI(t *testing.T) {
	testenv.Isolate(t)
	home := t.TempDir()
	t.Setenv("MINO_HOME", home)

	_ = build.KnownSignals()
	if err := config.SaveGlobalSettings(config.GlobalSettings{Onboarded: true}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("output: terminal\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	id := "mino.ntr"
	if _, ok := plugin.Lookup(id); !ok {
		t.Skip("mino.ntr not linked in this test binary")
	}

	run := func(args ...string) (string, error) {
		root := newRootCmd()
		root.SetArgs(args)
		var buf bytes.Buffer
		root.SetOut(&buf)
		root.SetErr(&buf)
		err := root.Execute()
		Shutdown()
		return buf.String(), err
	}

	out, err := run("plugins", "install", id)
	if err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	if !strings.Contains(out, "installed "+id) {
		t.Fatalf("install output: %s", out)
	}
	if _, err := os.Stat(filepath.Join(home, "queries", "ntr-list.yaml")); err != nil {
		t.Fatalf("seed missing: %v", err)
	}
	if !plugin.Enabled(id) {
		t.Fatal("expected enabled after install")
	}
	if !plugin.Installed(id) {
		t.Fatal("expected installed after install")
	}

	out, err = run("plugins", "list")
	if err != nil {
		t.Fatalf("list: %v\n%s", err, out)
	}
	if !strings.Contains(out, id) || !strings.Contains(out, "enabled") {
		t.Fatalf("list output: %s", out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	seenExternal := false
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		pid := fields[0]
		if plugin.IsInternal(pid) {
			if seenExternal {
				t.Fatalf("internal %q after external in plugins list:\n%s", pid, out)
			}
			continue
		}
		seenExternal = true
	}

	out, err = run("plugins", "uninstall", id)
	if err == nil {
		t.Fatalf("expected uninstall of built-in to fail:\n%s", out)
	}
	if !plugin.Installed(id) {
		t.Fatal("built-in must remain installed after rejected uninstall")
	}
	if !plugin.Enabled(id) {
		t.Fatal("built-in must remain enabled after rejected uninstall")
	}
	if _, err := os.Stat(filepath.Join(home, "queries", "ntr-list.yaml")); err != nil {
		t.Fatalf("seeds should remain after rejected uninstall: %v", err)
	}

	if out, err = run("plugins", "disable", id); err != nil || !strings.Contains(out, "disabled "+id) {
		t.Fatalf("disable: %v\n%s", err, out)
	}
	if out, err = run("plugins", "enable", id); err != nil || !strings.Contains(out, "enabled "+id) {
		t.Fatalf("enable: %v\n%s", err, out)
	}
}
