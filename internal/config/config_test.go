package config

import (
	"os"
	"path/filepath"
	"strings"
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
	if len(cfg.Plugins) != 0 {
		t.Errorf("plugin settings should default empty, got %#v", cfg.Plugins)
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
plugins:
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
	gmail := cfg.PluginSettings("gmail")
	if gmail["query"] != "is:starred" {
		t.Errorf("plugin settings not applied: %#v", gmail)
	}
	if len(cfg.PluginSettings("docs")) != 0 {
		t.Errorf("unconfigured plugin sections should stay empty, got %#v", cfg.PluginSettings("docs"))
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
	  "plugins": { "gmail": { "query": "is:starred", "max": 3 } }
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
	if got := cfg.PluginSettings("gmail")["max"]; got != float64(3) && got != 3 {
		t.Errorf("json plugins.gmail.max = %#v, want 3", got)
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

func TestResolveDetailTTL(t *testing.T) {
	cases := []struct {
		name          string
		local, global string
		want          string
	}{
		{"local wins", "90s", "10m", "90s"},
		{"global when no local", "", "10m", "10m"},
		{"default when neither", "", "", DefaultDetailTTL},
		{"explicit zero is honoured", "0", "10m", "0"},
		{"global zero is honoured", "", "0", "0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ResolveDetailTTL(c.local, c.global); got != c.want {
				t.Errorf("ResolveDetailTTL(%q, %q) = %q, want %q", c.local, c.global, got, c.want)
			}
		})
	}
}

func TestDefaultsLeaveDetailTTLUnsetSoTheGlobalSettingIsReachable(t *testing.T) {
	if got := Defaults().Cache.DetailTTL; got != "" {
		t.Fatalf("Defaults().Cache.DetailTTL = %q, want empty: a default here is always non-empty, so it would win in ResolveDetailTTL and detail_cache_ttl in settings.yaml could never take effect", got)
	}
	if got := ResolveDetailTTL(Defaults().Cache.DetailTTL, "10m"); got != "10m" {
		t.Errorf("with no local value the global should win, got %q", got)
	}
	if got := ResolveDetailTTL(Defaults().Cache.DetailTTL, ""); got != DefaultDetailTTL {
		t.Errorf("with neither set the built-in default should apply, got %q", got)
	}
}

func TestGlobalSettingsRoundTripsDetailCacheTTL(t *testing.T) {
	testenv.Isolate(t)
	if err := SaveGlobalSettings(GlobalSettings{DetailCacheTTL: "10m"}); err != nil {
		t.Fatal(err)
	}
	if got := LoadGlobalSettings().DetailCacheTTL; got != "10m" {
		t.Errorf("DetailCacheTTL = %q, want 10m", got)
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

func TestHiddenStatusBarPreference(t *testing.T) {
	testenv.Isolate(t)

	if StatusBarHidden("slack") {
		t.Fatal("expected slack visible by default")
	}
	if err := SetHiddenStatusBar([]string{"slack", "", "slack", "github"}); err != nil {
		t.Fatalf("SetHiddenStatusBar: %v", err)
	}
	if !StatusBarHidden("slack") || !StatusBarHidden("github") {
		t.Fatalf("expected slack/github hidden, got %#v", LoadGlobalSettings().HiddenStatusBar)
	}
	gs := LoadGlobalSettings()
	if len(gs.HiddenStatusBar) != 2 {
		t.Fatalf("dedupe failed: %#v", gs.HiddenStatusBar)
	}
	if err := SetHiddenStatusBar(nil); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if StatusBarHidden("slack") {
		t.Fatal("expected slack visible after clear")
	}
}

func TestHiddenStatusBarCollapsesLegacyGoogleKeys(t *testing.T) {
	testenv.Isolate(t)

	if err := SetHiddenStatusBar([]string{"gmail", "docs", "slack"}); err != nil {
		t.Fatalf("SetHiddenStatusBar: %v", err)
	}
	gs := LoadGlobalSettings()
	want := []string{"slack", "google"}
	if len(gs.HiddenStatusBar) != 2 || gs.HiddenStatusBar[0] != "slack" || gs.HiddenStatusBar[1] != "google" {
		t.Fatalf("normalize = %#v, want %#v", gs.HiddenStatusBar, want)
	}
	if !StatusBarHidden("google") {
		t.Fatal("expected google hidden via collapsed legacy keys")
	}
	if !StatusBarHidden("slack") {
		t.Fatal("expected slack still hidden")
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

func TestPluginSettingsEnvOverridesFileLeaves(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "config.yaml"), `
plugins:
  slack:
    token_env: FROM_FILE
    limit: 7
`)
	t.Setenv("MUNIN_PLUGINS_SLACK_TOKEN_ENV", "FROM_ENV")
	t.Setenv("MUNIN_PLUGINS_CALENDAR_MAX", "20")

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	slack := cfg.PluginSettings("slack")
	if slack["token_env"] != "FROM_ENV" {
		t.Errorf("token_env = %#v, want the env override to win over the file", slack["token_env"])
	}
	if slack["limit"] != 7 {
		t.Errorf("limit = %#v, want the file value 7 kept", slack["limit"])
	}
	for key := range slack {
		if strings.Contains(key, ".") {
			t.Errorf("settings carry a dotted key %q: multi-word leaves must land as snake_case", key)
		}
	}

	if got := cfg.PluginSettings("calendar")["max"]; got != "20" {
		t.Errorf("calendar.max = %#v, want the env override for a namespace absent from the file", got)
	}
}
