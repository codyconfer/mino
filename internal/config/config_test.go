package config

import (
	"os"
	"path/filepath"
	"testing"
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
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("MUNIN_HOME", "")

	if gs := LoadGlobalSettings(); gs.Home != "" || gs.Theme != "" {
		t.Errorf("missing settings should be zero, got %#v", gs)
	}

	wantHome := filepath.Join(t.TempDir(), "custom-munin")
	mkdir(t, filepath.Join(xdg, "munin"))
	write(t, filepath.Join(xdg, "munin", "settings.yaml"), "home: "+wantHome+"\ntheme: dracula\n")

	gs := LoadGlobalSettings()
	if gs.Home != wantHome {
		t.Errorf("global home = %q, want %q", gs.Home, wantHome)
	}
	if gs.Theme != "dracula" {
		t.Errorf("global theme = %q, want dracula", gs.Theme)
	}
	if p := GlobalSettingsPath(); p != filepath.Join(xdg, "munin", "settings.yaml") {
		t.Errorf("GlobalSettingsPath = %q", p)
	}
}

func TestHomePrecedence(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("MUNIN_HOME", "")

	if h, err := Home(""); err != nil || h == "" {
		t.Fatalf("default home = %q, %v", h, err)
	}

	mkdir(t, filepath.Join(xdg, "munin"))
	wantGlobal := filepath.Join(t.TempDir(), "custom-munin")
	write(t, filepath.Join(xdg, "munin", "settings.yaml"), "home: "+wantGlobal+"\n")
	if h, _ := Home(""); h != wantGlobal {
		t.Errorf("global home override = %q, want %q", h, wantGlobal)
	}

	if h, _ := Home("/explicit"); h != "/explicit" {
		t.Errorf("explicit override = %q, want /explicit", h)
	}

	t.Setenv("MUNIN_HOME", "/from-env")
	if h, _ := Home(""); h != "/from-env" {
		t.Errorf("env override = %q, want /from-env", h)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
