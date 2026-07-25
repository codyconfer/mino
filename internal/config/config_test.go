package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/codyconfer/munin/internal/testenv"
)

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Output != "terminal" {
		t.Errorf("default output = %q, want terminal", cfg.Output)
	}
	if !cfg.Audit.Enabled {
		t.Error("audit should default on")
	}
	if cfg.Slack.TokenEnv != "SLACK_TOKEN" || cfg.Cal.CalendarID != "primary" {
		t.Errorf("signal param defaults wrong: %+v %+v", cfg.Slack, cfg.Cal)
	}
	if cfg.Gmail.Max != 15 || cfg.Docs.Recent != 10 {
		t.Errorf("numeric defaults wrong: gmail.max=%d docs.recent=%d", cfg.Gmail.Max, cfg.Docs.Recent)
	}
	if cfg.Role != "" {
		t.Errorf("role should default empty, got %q", cfg.Role)
	}
	if cfg.Home != dir {
		t.Errorf("Home = %q, want %q", cfg.Home, dir)
	}
	want := DefaultKeybinds()
	if len(cfg.Keybinds) != len(want) {
		t.Fatalf("default keybinds = %#v, want %#v", cfg.Keybinds, want)
	}
	for k, v := range want {
		if cfg.Keybinds[k] != v {
			t.Errorf("keybinds[%q]=%q, want %q", k, cfg.Keybinds[k], v)
		}
	}
}

func TestLoadKeybindsOverride(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "config.yaml"), `
keybinds:
  alt+n: morning
  alt+x: ntr.note.new
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Keybinds["alt+n"] != "morning" {
		t.Errorf("alt+n = %q, want morning", cfg.Keybinds["alt+n"])
	}
	if cfg.Keybinds["alt+x"] != "ntr.note.new" {
		t.Errorf("alt+x = %q", cfg.Keybinds["alt+x"])
	}
}

func TestLoadFileOverrides(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "config.yaml"), `
output: json
role: triage
gmail:
  query: "is:starred"
  max: 3
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Output != "json" {
		t.Errorf("output = %q, want json", cfg.Output)
	}
	if cfg.Role != "triage" {
		t.Errorf("role = %q, want triage", cfg.Role)
	}
	if cfg.Gmail.Query != "is:starred" || cfg.Gmail.Max != 3 {
		t.Errorf("gmail overrides not applied: %+v", cfg.Gmail)
	}
	if cfg.Docs.Recent != 10 {
		t.Errorf("docs should retain its default (10), got %d", cfg.Docs.Recent)
	}
	if cfg.Keybinds["alt+n"] != "ntr.note.new" {
		t.Errorf("omitted keybinds should keep defaults, got %#v", cfg.Keybinds)
	}
}

func TestLoadConfigJSON(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "config.json"), `{
	  "output": "json",
	  "role": "oncall",
	  "gmail": { "query": "is:starred", "max": 3 }
	}`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Output != "json" {
		t.Errorf("output = %q, want json", cfg.Output)
	}
	if cfg.Role != "oncall" {
		t.Errorf("json role = %q, want oncall", cfg.Role)
	}
	if cfg.Gmail.Max != 3 {
		t.Errorf("json gmail.max = %d, want 3", cfg.Gmail.Max)
	}
}

func TestAuditDefaultsOn(t *testing.T) {

	if cfg, err := Load(t.TempDir()); err != nil || !cfg.Audit.Enabled {
		t.Fatalf("audit should default on with no file: enabled=%v err=%v", cfg.Audit.Enabled, err)
	}

	dir := t.TempDir()
	write(t, filepath.Join(dir, "config.yaml"), "output: json\n")
	if cfg, err := Load(dir); err != nil || !cfg.Audit.Enabled {
		t.Fatalf("audit should stay on when unset: enabled=%v err=%v", cfg.Audit.Enabled, err)
	}

	dir2 := t.TempDir()
	write(t, filepath.Join(dir2, "config.yaml"), "audit:\n  enabled: false\n")
	if cfg, _ := Load(dir2); cfg.Audit.Enabled {
		t.Error("audit.enabled:false should turn it off")
	}
}

func TestEnvOverride(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "config.yaml"), "output: terminal\n")
	t.Setenv("MUNIN_OUTPUT", "json")
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Output != "json" {
		t.Errorf("env should override file: output = %q, want json", cfg.Output)
	}
}

func TestLoadGlobalSettings(t *testing.T) {
	env := testenv.Isolate(t)

	if gs := LoadGlobalSettings(); gs.Home != "" || gs.Theme != "" {
		t.Errorf("missing settings should be zero, got %#v", gs)
	}

	wantHome := filepath.Join(t.TempDir(), "custom-munin")
	mkdir(t, filepath.Join(env.ConfigDir, "munin"))
	write(t, filepath.Join(env.ConfigDir, "munin", "settings.yaml"), "home: "+wantHome+"\ntheme: dracula\n")

	gs := LoadGlobalSettings()
	if gs.Home != wantHome {
		t.Errorf("global home = %q, want %q", gs.Home, wantHome)
	}
	if gs.Theme != "dracula" {
		t.Errorf("global theme = %q, want dracula", gs.Theme)
	}
	if p := GlobalSettingsPath(); p != filepath.Join(env.ConfigDir, "munin", "settings.yaml") {
		t.Errorf("GlobalSettingsPath = %q", p)
	}
}

func TestHomePrecedence(t *testing.T) {
	env := testenv.Isolate(t)

	wantDefault := filepath.Join(env.Home, HomeDirName)
	if h, err := Home(""); err != nil || h != wantDefault {
		t.Fatalf("default home = %q, %v; want %q", h, err, wantDefault)
	}
	if h, err := DefaultHome(); err != nil || h != wantDefault {
		t.Fatalf("DefaultHome = %q, %v; want %q", h, err, wantDefault)
	}

	mkdir(t, filepath.Join(env.ConfigDir, "munin"))
	wantGlobal := filepath.Join(t.TempDir(), "custom-munin")
	write(t, filepath.Join(env.ConfigDir, "munin", "settings.yaml"), "home: "+wantGlobal+"\n")
	if h, _ := Home(""); h != wantGlobal {
		t.Errorf("global home override = %q, want %q", h, wantGlobal)
	}

	wantExplicit := filepath.Join(t.TempDir(), "explicit")
	if h, _ := Home(wantExplicit); h != wantExplicit {
		t.Errorf("explicit override = %q, want %q", h, wantExplicit)
	}

	wantEnv := filepath.Join(t.TempDir(), "from-env")
	t.Setenv("MUNIN_HOME", wantEnv)
	if h, _ := Home(""); h != wantEnv {
		t.Errorf("env override = %q, want %q", h, wantEnv)
	}
}

func TestHomeResolvesRelativeToAbsolute(t *testing.T) {
	testenv.Isolate(t)
	cwd := t.TempDir()
	t.Chdir(cwd)

	got, err := Home(".munin-rel")
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs(filepath.Join(cwd, ".munin-rel"))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Home(.munin-rel) = %q, want absolute %q", got, want)
	}
}

func TestHomeExpandsTilde(t *testing.T) {
	env := testenv.Isolate(t)

	got, err := Home("~/alt-munin")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(env.Home, "alt-munin")
	if got != want {
		t.Fatalf("tilde home = %q, want %q", got, want)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
