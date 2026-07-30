package config

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/codyconfer/munin/internal/log"
	"github.com/codyconfer/munin/internal/testenv"
)

const malformedSettings = "home: /work/munin\n\ttheme: dracula\n"

func settingsPath(t *testing.T, env testenv.Env) string {
	t.Helper()
	mkdir(t, filepath.Join(env.ConfigDir, "munin"))
	return filepath.Join(env.ConfigDir, "munin", "settings.yaml")
}

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

func TestLoadGlobalSettingsWarnsInsteadOfSilentlyReturningZeroForMalformedFile(t *testing.T) {
	env := testenv.Isolate(t)
	path := settingsPath(t, env)
	write(t, path, malformedSettings)
	logs := captureLog(t)

	LoadGlobalSettings()

	got := logs.String()
	if got == "" {
		t.Fatalf("a malformed %s was treated as empty settings with no diagnostic at all; munin would silently use a different home directory", path)
	}
	if !strings.Contains(got, path) {
		t.Errorf("warning should name the offending file %q, got %q", path, got)
	}
}

func TestReadGlobalSettingsReportsParseFailure(t *testing.T) {
	env := testenv.Isolate(t)
	path := settingsPath(t, env)
	write(t, path, malformedSettings)

	gs, err := ReadGlobalSettings()
	if err == nil {
		t.Fatalf("expected a parse error for malformed settings, got %#v", gs)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error should name the offending file %q, got %v", path, err)
	}
}

func TestSaveGlobalSettingsRefusesToOverwriteUnparseableFile(t *testing.T) {
	env := testenv.Isolate(t)
	path := settingsPath(t, env)
	write(t, path, malformedSettings)

	gs := LoadGlobalSettings()
	if err := SaveGlobalSettings(gs); err == nil {
		t.Error("SaveGlobalSettings overwrote settings munin could not parse; the user's real home and plugin list are gone")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != malformedSettings {
		t.Fatalf("settings file was rewritten:\n got %q\nwant %q", string(data), malformedSettings)
	}
}

func TestSetHiddenStatusBarDoesNotDestroyUnparseableSettings(t *testing.T) {
	env := testenv.Isolate(t)
	path := settingsPath(t, env)
	original := "home: /work/munin\ninstalled_plugins:\n  - jira\n\ttheme: dracula\n"
	write(t, path, original)

	if err := SetHiddenStatusBar([]string{"slack"}); err == nil {
		t.Error("a status-bar toggle silently rewrote an unparseable settings file")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf("settings file was rewritten:\n got %q\nwant %q", string(data), original)
	}
}

func TestSaveGlobalSettingsWorksWhenFileIsAbsent(t *testing.T) {
	testenv.Isolate(t)

	if gs := LoadGlobalSettings(); !reflect.DeepEqual(gs, GlobalSettings{}) {
		t.Fatalf("absent settings should load as zero, got %#v", gs)
	}
	gs, err := ReadGlobalSettings()
	if err != nil {
		t.Fatalf("absent settings should not be an error: %v", err)
	}
	if !reflect.DeepEqual(gs, GlobalSettings{}) {
		t.Fatalf("absent settings should be zero, got %#v", gs)
	}

	home := filepath.Join(t.TempDir(), "custom-munin")
	if err := SaveGlobalSettings(GlobalSettings{Home: home, Theme: "dracula"}); err != nil {
		t.Fatalf("SaveGlobalSettings on an absent file: %v", err)
	}
	if got := LoadGlobalSettings(); got.Home != home || got.Theme != "dracula" {
		t.Fatalf("round trip = %#v", got)
	}
}

func TestSaveGlobalSettingsStillOverwritesParseableFile(t *testing.T) {
	env := testenv.Isolate(t)
	path := settingsPath(t, env)
	write(t, path, "home: /work/munin\ntheme: dracula\n")

	gs := LoadGlobalSettings()
	if gs.Home != "/work/munin" {
		t.Fatalf("valid settings failed to load: %#v", gs)
	}
	gs.Theme = "nord"
	if err := SaveGlobalSettings(gs); err != nil {
		t.Fatalf("SaveGlobalSettings on a valid file: %v", err)
	}
	if got := LoadGlobalSettings(); got.Theme != "nord" || got.Home != "/work/munin" {
		t.Fatalf("round trip = %#v", got)
	}
}

func TestLoadGlobalSettingsDoesNotWarnForValidOrAbsentFiles(t *testing.T) {
	env := testenv.Isolate(t)
	logs := captureLog(t)

	LoadGlobalSettings()
	write(t, settingsPath(t, env), "home: /work/munin\n")
	LoadGlobalSettings()

	if got := logs.String(); got != "" {
		t.Errorf("unexpected log output for healthy settings: %q", got)
	}
}

func TestSaveGlobalSettingsDoesNotWriteThroughASymlink(t *testing.T) {
	env := testenv.Isolate(t)
	path := settingsPath(t, env)
	victim := filepath.Join(t.TempDir(), "victim.yaml")
	original := "home: /elsewhere\n"
	write(t, victim, original)
	if err := os.Symlink(victim, path); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}

	if err := SaveGlobalSettings(GlobalSettings{Theme: "nord"}); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(victim); err != nil || string(raw) != original {
		t.Fatalf("a file outside the config dir was overwritten through a symlink: %q (err=%v)", raw, err)
	}
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("%s is still a symlink; settings writes still escape the config dir", path)
	}
}
