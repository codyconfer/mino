package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/codyconfer/munin/internal/testenv"
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
		{RelPath: "queries/test-install.yaml", Content: []byte("name: test-install\nsignal: testinstall\n")},
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
	if string(got) != "name: test-install\nsignal: testinstall\n" {
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
	content := []byte("name: u\nsignal: testuninstall\n")
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

func TestInstallUnknownPlugin(t *testing.T) {
	testenv.Isolate(t)
	_, err := Install(t.TempDir(), "does.not.exist", InstallOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestInternalPluginsAlwaysInstalled(t *testing.T) {
	testenv.Isolate(t)
	id := "munin.demo"
	if _, ok := Lookup(id); !ok {
		RegisterBuiltins()
	}
	if _, ok := Lookup(id); !ok {
		t.Skip("munin.demo not registered")
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
