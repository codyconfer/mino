package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/testenv"
)

func TestInstallEnablesAndWritesSeeds(t *testing.T) {
	testenv.Isolate(t)
	id := "test.install.plugin"
	if _, ok := Lookup(id); !ok {
		Register(Descriptor{
			ID:           id,
			Kind:         KindSignal,
			Signal:       "testinstall",
			Capabilities: []Capability{CapQuery},
		})
	}
	RegisterSeeds(id, []FileSeed{
		{RelPath: "queries/test-install.yaml", Content: []byte("name: test-install\ntype: query\nsignal: testinstall\n")},
	})
	t.Cleanup(func() { RegisterSeeds(id, nil) })

	home := t.TempDir()
	res, err := Install(home, id, InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Installed || !Installed(id) {
		t.Fatalf("expected plugin installed: %+v", res)
	}
	if !res.Enabled || !Enabled(id) {
		t.Fatalf("expected plugin enabled: %+v", res)
	}
	if len(res.Written) != 1 || res.Written[0] != "queries/test-install.yaml" {
		t.Fatalf("written = %v", res.Written)
	}
	got, err := os.ReadFile(filepath.Join(home, "queries", "test-install.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "name: test-install\ntype: query\nsignal: testinstall\n" {
		t.Fatalf("content = %q", got)
	}

	res2, err := Install(home, id, InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.Written) != 0 || len(res2.Skipped) != 1 {
		t.Fatalf("second install: %+v", res2)
	}
}

func TestInstallForceOverwrites(t *testing.T) {
	testenv.Isolate(t)
	id := "test.install.force"
	if _, ok := Lookup(id); !ok {
		Register(Descriptor{ID: id, Kind: KindSignal, Signal: "testforce", Capabilities: []Capability{CapQuery}})
	}
	RegisterSeeds(id, []FileSeed{
		{RelPath: "queries/force.yaml", Content: []byte("v2\n")},
	})
	t.Cleanup(func() { RegisterSeeds(id, nil) })

	home := t.TempDir()
	path := filepath.Join(home, "queries", "force.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("v1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := Install(home, id, InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Skipped) != 1 || len(res.Written) != 0 {
		t.Fatalf("without force expected skip: %+v", res)
	}

	res, err = Install(home, id, InstallOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Written) != 1 {
		t.Fatalf("force write: %+v", res)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "v2\n" {
		t.Fatalf("got %q", got)
	}
}

func TestUninstallDisablesAndRemovesMatchingSeeds(t *testing.T) {
	testenv.Isolate(t)
	id := "test.uninstall.plugin"
	if _, ok := Lookup(id); !ok {
		Register(Descriptor{ID: id, Kind: KindSignal, Signal: "testuninstall", Capabilities: []Capability{CapQuery}})
	}
	content := []byte("name: u\ntype: query\nsignal: testuninstall\n")
	RegisterSeeds(id, []FileSeed{{RelPath: "queries/u.yaml", Content: content}})
	t.Cleanup(func() { RegisterSeeds(id, nil) })

	home := t.TempDir()
	if _, err := Install(home, id, InstallOptions{}); err != nil {
		t.Fatal(err)
	}

	res, err := Uninstall(home, id, UninstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Uninstalled || Installed(id) {
		t.Fatalf("expected uninstalled: %+v installed=%v", res, Installed(id))
	}
	if !res.Disabled || Enabled(id) {
		t.Fatalf("expected disabled: %+v enabled=%v", res, Enabled(id))
	}
	if len(res.Removed) != 1 {
		t.Fatalf("removed = %v", res.Removed)
	}
	if _, err := os.Stat(filepath.Join(home, "queries", "u.yaml")); !os.IsNotExist(err) {
		t.Fatalf("file should be gone: %v", err)
	}
}

func TestDisableKeepsInstalledUninstallClears(t *testing.T) {
	testenv.Isolate(t)
	id := "test.disable.vs.uninstall"
	if _, ok := Lookup(id); !ok {
		Register(Descriptor{ID: id, Kind: KindSignal, Signal: "testdvu", Capabilities: []Capability{CapQuery}})
	}
	home := t.TempDir()
	if _, err := Install(home, id, InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := SetEnabled(id, false); err != nil {
		t.Fatal(err)
	}
	if !Installed(id) || Enabled(id) {
		t.Fatalf("disable should keep installed: installed=%v enabled=%v", Installed(id), Enabled(id))
	}
	listed := ListInstalled()
	found := false
	for _, row := range listed {
		if row.ID == id {
			found = true
			if row.Enabled {
				t.Fatal("ListInstalled should report disabled")
			}
		}
	}
	if !found {
		t.Fatal("disabled plugin missing from ListInstalled")
	}
	if _, err := Uninstall(home, id, UninstallOptions{KeepSeeds: true}); err != nil {
		t.Fatal(err)
	}
	if Installed(id) {
		t.Fatal("uninstall should clear installed")
	}
	for _, row := range ListInstalled() {
		if row.ID == id {
			t.Fatal("uninstalled plugin still in ListInstalled")
		}
	}
	if _, err := Install(home, id, InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	if !Installed(id) || !Enabled(id) {
		t.Fatalf("reinstall: installed=%v enabled=%v", Installed(id), Enabled(id))
	}
}

func TestUninstallKeepsModifiedSeeds(t *testing.T) {
	testenv.Isolate(t)
	id := "test.uninstall.keep"
	if _, ok := Lookup(id); !ok {
		Register(Descriptor{ID: id, Kind: KindSignal, Signal: "testkeep", Capabilities: []Capability{CapQuery}})
	}
	RegisterSeeds(id, []FileSeed{{RelPath: "queries/k.yaml", Content: []byte("catalog\n")}})
	t.Cleanup(func() { RegisterSeeds(id, nil) })

	home := t.TempDir()
	if _, err := Install(home, id, InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "queries", "k.yaml")
	if err := os.WriteFile(path, []byte("user-edit\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := Uninstall(home, id, UninstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Kept) != 1 || len(res.Removed) != 0 {
		t.Fatalf("expected keep modified: %+v", res)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "user-edit\n" {
		t.Fatalf("file mutated: %q", got)
	}

	res, err = Uninstall(home, id, UninstallOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Removed) != 1 {
		t.Fatalf("force remove: %+v", res)
	}
}

func TestUninstallKeepSeeds(t *testing.T) {
	testenv.Isolate(t)
	id := "test.uninstall.keepseeds"
	if _, ok := Lookup(id); !ok {
		Register(Descriptor{ID: id, Kind: KindSignal, Signal: "testkeepseeds", Capabilities: []Capability{CapQuery}})
	}
	RegisterSeeds(id, []FileSeed{{RelPath: "queries/ks.yaml", Content: []byte("x\n")}})
	t.Cleanup(func() { RegisterSeeds(id, nil) })

	home := t.TempDir()
	if _, err := Install(home, id, InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	res, err := Uninstall(home, id, UninstallOptions{KeepSeeds: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Uninstalled || Installed(id) {
		t.Fatalf("keep-seeds should uninstall: %+v installed=%v", res, Installed(id))
	}
	if !res.Disabled || len(res.Removed) != 0 || len(res.Kept) != 1 {
		t.Fatalf("keep-seeds: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(home, "queries", "ks.yaml")); err != nil {
		t.Fatal(err)
	}
}

func escapeHomeFixture(t *testing.T) (home, victim string) {
	t.Helper()
	base := t.TempDir()
	home = filepath.Join(base, "nest", "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	victim = filepath.Join(base, "nest", "victim.yaml")
	return home, victim
}

const escapeVictimData = "important: user data\n"

func TestInstallSeedCannotEscapeHome(t *testing.T) {
	testenv.Isolate(t)
	id := "test.install.escape"
	if _, ok := Lookup(id); !ok {
		Register(Descriptor{ID: id, Kind: KindSignal, Signal: "testinstallescape", Capabilities: []Capability{CapQuery}})
	}
	home, victim := escapeHomeFixture(t)
	if err := os.WriteFile(victim, []byte(escapeVictimData), 0o600); err != nil {
		t.Fatal(err)
	}

	RegisterSeeds(id, []FileSeed{
		{RelPath: "../victim.yaml", Content: []byte("evil\n")},
		{RelPath: "../../.config/systemd/user/evil.service", Content: []byte("evil\n")},
	})
	t.Cleanup(func() { RegisterSeeds(id, nil) })

	res, _ := Install(home, id, InstallOptions{Force: true})
	for _, w := range res.Written {
		if strings.Contains(w, "..") {
			t.Errorf("Install reported writing outside the home: %q", w)
		}
	}
	got, err := os.ReadFile(victim)
	if err != nil {
		t.Fatalf("file outside the mino home was removed/renamed: %v", err)
	}
	if string(got) != escapeVictimData {
		t.Fatalf("install clobbered a file outside the mino home: %q", got)
	}
	outsideTree := filepath.Join(home, "..", "..", ".config")
	if _, err := os.Stat(outsideTree); !os.IsNotExist(err) {
		t.Fatalf("install created %s outside the mino home: %v", outsideTree, err)
	}
}

func TestUninstallSeedRemoveCannotEscapeHome(t *testing.T) {
	testenv.Isolate(t)
	id := "test.uninstall.escape"
	if _, ok := Lookup(id); !ok {
		Register(Descriptor{ID: id, Kind: KindSignal, Signal: "testuninstallescape", Capabilities: []Capability{CapQuery}})
	}
	home, victim := escapeHomeFixture(t)
	seed := []byte("seed body\n")
	if err := os.WriteFile(victim, seed, 0o600); err != nil {
		t.Fatal(err)
	}

	RegisterSeeds(id, []FileSeed{{RelPath: "../victim.yaml", Content: seed}})
	t.Cleanup(func() { RegisterSeeds(id, nil) })

	res, _ := Uninstall(home, id, UninstallOptions{Force: true})
	for _, r := range res.Removed {
		if strings.Contains(r, "..") {
			t.Errorf("Uninstall reported removing outside the home: %q", r)
		}
	}
	if _, err := os.Stat(victim); err != nil {
		t.Fatalf("uninstall removed a file outside the mino home: %v", err)
	}
}

func TestWriteSeedsRefusesEscapingRelPath(t *testing.T) {
	home, victim := escapeHomeFixture(t)
	if err := os.WriteFile(victim, []byte(escapeVictimData), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"../victim.yaml", "queries/../../victim.yaml", filepath.Join(filepath.Dir(home), "victim.yaml")} {
		written, _, err := writeSeeds(home, []FileSeed{{RelPath: rel, Content: []byte("evil\n")}}, InstallOptions{Force: true})
		if err == nil {
			t.Errorf("writeSeeds accepted %q (written=%v)", rel, written)
		}
		got, rerr := os.ReadFile(victim)
		if rerr != nil {
			t.Fatalf("%q: victim gone: %v", rel, rerr)
		}
		if string(got) != escapeVictimData {
			t.Fatalf("%q clobbered a file outside the home: %q", rel, got)
		}
	}
}

func TestRemoveSeedsRefusesEscapingRelPath(t *testing.T) {
	home, victim := escapeHomeFixture(t)
	body := []byte("seed body\n")
	if err := os.WriteFile(victim, body, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"../victim.yaml", "queries/../../victim.yaml", filepath.Join(filepath.Dir(home), "victim.yaml"), `..\victim.yaml`, "..", ""} {
		removed, _, err := removeSeeds(home, []FileSeed{{RelPath: rel, Content: body}}, true)
		if err == nil {
			t.Errorf("removeSeeds accepted %q (removed=%v)", rel, removed)
		}
		if _, err := os.Stat(victim); err != nil {
			t.Fatalf("%q removed a file outside the home: %v", rel, err)
		}
	}
}

func TestSeedTargetContainment(t *testing.T) {
	home := t.TempDir()
	for _, rel := range []string{"", "   ", "..", "../x", "queries/../../x", "/etc/x", `..\x`, `..\..\x`, "."} {
		if got, err := seedTarget(home, rel); err == nil {
			t.Errorf("seedTarget(%q) = %q, want error", rel, got)
		}
	}
	got, err := seedTarget(home, "queries/x.yaml")
	if err != nil || got != filepath.Join(home, "queries", "x.yaml") {
		t.Fatalf("seedTarget = %q, %v", got, err)
	}
}

func TestInstallUnknownPlugin(t *testing.T) {
	testenv.Isolate(t)
	_, err := Install(t.TempDir(), "does.not.exist", InstallOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestInternalPluginsAlwaysInstalled(t *testing.T) {
	testenv.Isolate(t)
	id := "mino.demo"
	if _, ok := Lookup(id); !ok {
		RegisterBuiltins()
	}
	if _, ok := Lookup(id); !ok {
		t.Skip("mino.demo not registered")
	}
	LoadEnabled()
	if !Installed(id) {
		t.Fatal("internal plugin must be installed without installed_plugins entry")
	}
	listed := false
	for _, row := range ListInstalled() {
		if row.ID == id {
			listed = true
			if !row.Enabled {
				t.Fatal("internal defaults to enabled")
			}
		}
	}
	if !listed {
		t.Fatal("internal missing from ListInstalled")
	}
	if _, err := Uninstall(t.TempDir(), id, UninstallOptions{KeepSeeds: true}); err == nil {
		t.Fatal("uninstall of built-in must fail")
	}
	if !Installed(id) {
		t.Fatal("failed uninstall must leave internal installed")
	}
	if _, err := Install(t.TempDir(), id, InstallOptions{}); err != nil {
		t.Fatalf("install seeds for built-in: %v", err)
	}
	if !Installed(id) || !Enabled(id) {
		t.Fatalf("after seed install: installed=%v enabled=%v", Installed(id), Enabled(id))
	}
}

func TestStockSeedsMatchExamples(t *testing.T) {
	root := filepath.Join("..", "..", "examples")
	for _, id := range SeedPluginIDs() {
		for _, seed := range SeedsFor(id) {
			path := filepath.Join(root, filepath.FromSlash(seed.RelPath))
			want, err := os.ReadFile(path)
			if err != nil {
				t.Errorf("%s: missing examples/%s: %v", id, seed.RelPath, err)
				continue
			}
			if string(want) != string(seed.Content) {
				t.Errorf("%s: examples/%s drifted from catalog\n--- examples\n%s\n--- catalog\n%s",
					id, seed.RelPath, want, seed.Content)
			}
		}
	}
}

func TestStockSeedsDeclareATypeAndLoad(t *testing.T) {
	home := t.TempDir()
	var seen int
	for _, id := range SeedPluginIDs() {
		for _, seed := range SeedsFor(id) {
			if !strings.Contains(string(seed.Content), "type:") {
				t.Errorf("%s: seed %s declares no type:\n%s", id, seed.RelPath, seed.Content)
			}
			dest := filepath.Join(home, filepath.FromSlash(seed.RelPath))
			if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dest, seed.Content, 0o600); err != nil {
				t.Fatal(err)
			}
			seen++
		}
	}
	if seen == 0 {
		t.Skip("no stock seeds registered")
	}
	if _, err := config.LoadDirectivesFromFiles(home); err != nil {
		t.Fatalf("stock seeds do not load as directives: %v", err)
	}
}

func TestSetEnabledRoundTrip(t *testing.T) {
	testenv.Isolate(t)
	id := "test.enable.roundtrip"
	if _, ok := Lookup(id); !ok {
		Register(Descriptor{ID: id, Kind: KindSignal, Signal: "testenable", Capabilities: []Capability{CapQuery}})
	}
	if err := SetEnabled(id, false); err != nil {
		t.Fatal(err)
	}
	if Enabled(id) {
		t.Fatal("expected disabled")
	}
	if err := SetEnabled(id, true); err != nil {
		t.Fatal(err)
	}
	if !Enabled(id) {
		t.Fatal("expected enabled")
	}
}
