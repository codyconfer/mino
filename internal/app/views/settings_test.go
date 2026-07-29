package views

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/render/glyph"
)

func settingsLabels(kit *Kit) []string {
	labels := make([]string, 0, len(kit.settingsMenuItems()))
	for _, it := range kit.settingsMenuItems() {
		labels = append(labels, it.Label)
	}
	return labels
}

func setvRender(v vkdeck.View) string {
	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	return app.View()
}

func setvFormValues(v vkdeck.View) map[string]any {
	return v.(*vkdeck.FormView).Values()
}

func setvFormKeys(t *testing.T, v vkdeck.View) map[string]struct{} {
	t.Helper()
	vals := setvFormValues(v)
	out := make(map[string]struct{}, len(vals))
	for k := range vals {
		out[k] = struct{}{}
	}
	return out
}

func setvWantKeys(t *testing.T, v vkdeck.View, want ...string) {
	t.Helper()
	got := setvFormKeys(t, v)
	for _, w := range want {
		if _, ok := got[w]; !ok {
			t.Errorf("form %q missing field %q: %v", v.Title(), w, got)
		}
	}
}

func setvStack(kit *Kit, form vkdeck.View) *vkdeck.Model {
	app := deck.New(kit.Settings())
	_ = app.Push(form)
	return app
}

func setvBody(app *vkdeck.Model) string {
	return ansi.Strip(step(app, tea.WindowSizeMsg{Width: 120, Height: 40}).View())
}

func setvBreakConfigDir(t *testing.T) {
	t.Helper()
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", blocker)
	t.Setenv("AppData", blocker)
	t.Setenv("HOME", blocker)
	t.Setenv("USERPROFILE", blocker)
}

func setvKeepAppearance(t *testing.T) {
	t.Helper()
	th, sc := *theme.Cur(), keys.Cur()
	t.Cleanup(func() {
		theme.Use(th)
		keys.Use(sc)
	})
}

func hasLabel(labels []string, want string) bool {
	for _, l := range labels {
		if l == want {
			return true
		}
	}
	return false
}

func TestSettingsMenuHidesCreateWhenConfigExists(t *testing.T) {
	kit := testKit(t)
	labels := settingsLabels(kit)
	if !hasLabel(labels, "Create config") {
		t.Fatalf("expected Create config when no file: %v", labels)
	}
	if hasLabel(labels, "Delete config") || hasLabel(labels, "Import config") {
		t.Fatalf("Delete/Import should be hidden without a config file: %v", labels)
	}

	if err := os.WriteFile(filepath.Join(kit.d.App.Cfg.Home, "config.yaml"), []byte("output: terminal\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	labels = settingsLabels(kit)
	if hasLabel(labels, "Create config") {
		t.Fatalf("Create config should be hidden when file exists: %v", labels)
	}
	if !hasLabel(labels, "Delete config") || !hasLabel(labels, "Import config") {
		t.Fatalf("expected Delete/Import when config exists: %v", labels)
	}
}

func TestSettingsMenuNamingAndExportAlwaysPresent(t *testing.T) {
	kit := testKit(t)
	labels := settingsLabels(kit)
	for _, want := range []string{"Edit config", "Create config", "Export config", "Open config in editor", "Appearance", "Status bar"} {
		if !hasLabel(labels, want) {
			t.Fatalf("settings missing %q: %v", want, labels)
		}
	}
	for _, bad := range []string{"Create config file", "Overwrite DuckDB with file", "Export DuckDB → files", "Show active config"} {
		if hasLabel(labels, bad) {
			t.Fatalf("settings still uses old label %q: %v", bad, labels)
		}
	}

	app := deck.New(kit.Settings())
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	body := app.View()
	if !strings.Contains(strings.ToUpper(body), "SETTINGS") {
		t.Fatalf("settings title missing: %q", body)
	}
	if strings.Contains(body, "settings tools") || strings.Contains(body, "SETTINGS TOOLS") {
		t.Fatalf("old settings title still present: %q", body)
	}
}

func TestSettingsStatusBarFormTogglesVisibility(t *testing.T) {
	kit := testKit(t)

	info := deck.StatusInfo{Services: []deck.ServiceStatus{
		{Name: "github", Level: deck.StatusOK},
		{Name: "slack", Level: deck.StatusOK},
	}}
	app := deck.New(kit.setvStatusBarView(), deck.WithStatus(func(context.Context) deck.StatusInfo {
		return info
	}))
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	if cmd := app.RefreshStatus(); cmd != nil {
		app = step(app, cmd())
	}
	chrome := ansi.Strip(app.View())
	if !strings.Contains(chrome, glyph.GitHub()) || !strings.Contains(chrome, glyph.Slack()) {
		t.Fatalf("expected github+slack chips before hide:\n%s", chrome)
	}

	body := app.View()
	if !strings.Contains(strings.ToUpper(body), "STATUS BAR") {
		t.Fatalf("status bar title missing: %q", body)
	}
	if !strings.Contains(body, "github") || !strings.Contains(body, "slack") || !strings.Contains(body, "google") {
		t.Fatalf("expected builtin toggles: %q", body)
	}
	for _, old := range []string{"calendar", "gmail", "docs", "drive"} {
		if strings.Contains(body, old) {
			t.Fatalf("status bar UI still lists per-signal %q: %q", old, body)
		}
	}
	if strings.Contains(strings.ToLower(body), "uninstall") {
		t.Fatalf("status bar UI should not suggest uninstall: %q", body)
	}

	app = step(app, tea.KeyMsg{Type: tea.KeyLeft})
	app, cmd := update(app, tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil {
		for _, c := range flattenCmds(cmd) {
			if c == nil {
				continue
			}
			app = step(app, c())
		}
	}
	if !config.StatusBarHidden("github") {
		t.Fatalf("expected github hidden after save, got %#v", config.LoadGlobalSettings().HiddenStatusBar)
	}
	chrome = ansi.Strip(app.View())
	if strings.Contains(chrome, glyph.GitHub()) {
		t.Fatalf("expected github chip gone after status-bar save refresh:\n%s", chrome)
	}
	if !strings.Contains(chrome, glyph.Slack()) {
		t.Fatalf("expected slack chip to remain after hide:\n%s", chrome)
	}
}

func TestSettingsEditConfigWritesHome(t *testing.T) {
	kit := testKit(t)
	home := kit.d.App.Cfg.Home
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("output: terminal\ntimeout: 30s\naudit:\n  enabled: false\nbackup:\n  destination: local\n  keep: 1\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	app := deck.New(kit.setvEditConfigView())
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	app = step(app, tea.KeyMsg{Type: tea.KeyRight})
	app, cmd := update(app, tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil {
		for _, c := range flattenCmds(cmd) {
			if c == nil {
				continue
			}
			app = step(app, c())
		}
	}
	raw, err := os.ReadFile(filepath.Join(home, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "output: json") {
		t.Fatalf("config file not updated: %s", raw)
	}
}

func TestSettingsOpenConfigInEditorRequiresFileAndEditor(t *testing.T) {
	kit := testKit(t)
	app := deck.New(kit.Settings())

	var open func(*vkdeck.Model) tea.Cmd
	for _, it := range kit.settingsMenuItems() {
		if it.Label == "Open config in editor" {
			open = it.Do
			break
		}
	}
	if open == nil {
		t.Fatal("Open config in editor menu item missing")
	}

	_ = open(app)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	got := app.View()
	if !strings.Contains(strings.ToUpper(got), "OPEN CONFIG") {
		t.Fatalf("expected open config error view: %q", got)
	}
	if !strings.Contains(strings.ToLower(got), "no config file") {
		t.Fatalf("expected missing-file error: %q", got)
	}

	if err := os.WriteFile(filepath.Join(kit.d.App.Cfg.Home, "config.yaml"), []byte("output: terminal\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	app = deck.New(kit.Settings())
	_ = open(app)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	got = app.View()
	if !strings.Contains(got, "EDITOR not set") {
		t.Fatalf("expected EDITOR-not-set error: %q", got)
	}

	t.Setenv("EDITOR", "true")
	app = deck.New(kit.Settings())
	_ = open(app)
	if title := app.Top().Title(); title != "open config" {
		t.Fatalf("Top title = %q, want open config editor view", title)
	}
}

func TestSettingsImportExportRequireConfirmation(t *testing.T) {
	kit := testKit(t)
	if err := os.WriteFile(filepath.Join(kit.d.App.Cfg.Home, "config.yaml"), []byte("output: terminal\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	app := deck.New(kit.Settings())
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})

	app = step(app, tea.KeyMsg{Type: tea.KeyDown})
	app = step(app, tea.KeyMsg{Type: tea.KeyDown})
	app, cmd := update(app, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		for _, c := range flattenCmds(cmd) {
			if c == nil {
				continue
			}
			app = step(app, c())
		}
	}
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	got := app.View()
	if !strings.Contains(strings.ToUpper(got), "IMPORT CONFIG") {
		t.Fatalf("import confirm title missing: %q", got)
	}
	if !strings.Contains(got, "Yes, import") || !strings.Contains(got, "No, cancel") {
		t.Fatalf("import confirm options missing: %q", got)
	}

	app = step(app, tea.KeyMsg{Type: tea.KeyEsc})
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})

	app = step(app, tea.KeyMsg{Type: tea.KeyDown})
	app, cmd = update(app, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		for _, c := range flattenCmds(cmd) {
			if c == nil {
				continue
			}
			app = step(app, c())
		}
	}
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	got = app.View()
	if !strings.Contains(strings.ToUpper(got), "EXPORT CONFIG") {
		t.Fatalf("export confirm title missing: %q", got)
	}
	if !strings.Contains(got, "Yes, export") || !strings.Contains(got, "No, cancel") {
		t.Fatalf("export confirm options missing: %q", got)
	}
}

func TestSetvEditConfigFormFields(t *testing.T) {
	kit := testKit(t)
	v := kit.setvEditConfigView()
	setvWantKeys(t, v, "output", "audit.enabled", "timeout", "backup.destination", "backup.keep")
	if got := v.Title(); got != "edit config" {
		t.Errorf("title = %q, want edit config", got)
	}
	if got := v.Hints(); len(got) != 3 || got[2][0] != "ctrl+s" {
		t.Errorf("hints = %v, want explicit field/change/ctrl+s legend", got)
	}
	for _, h := range v.Hints() {
		if strings.ContainsAny(h[0], "jk") {
			t.Errorf("hints advertise unbound single-char keys: %v", v.Hints())
		}
	}
}

func TestSetvSaveConfigPersistsValues(t *testing.T) {
	kit := testKit(t)
	home := kit.d.App.Cfg.Home
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("output: terminal\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	app := setvStack(kit, kit.setvEditConfigView())
	vals := map[string]any{
		"output":             "json",
		"timeout":            "45s",
		"audit.enabled":      true,
		"backup.destination": "gdrive",
		"backup.keep":        "7",
	}
	if cmd := kit.setvSaveConfig(app, vals); cmd == nil {
		t.Fatal("setvSaveConfig returned nil cmd")
	}
	raw, err := os.ReadFile(filepath.Join(home, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"output: json", "timeout: 45s", "enabled: true", "destination: gdrive", "keep: 7"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("config missing %q:\n%s", want, raw)
		}
	}
	if got := app.Top().Title(); got != "edit config" {
		t.Fatalf("Top title = %q, want edit config", got)
	}
	if body := setvBody(app); !strings.Contains(body, "wrote ") {
		t.Fatalf("expected wrote-path message: %q", body)
	}
}

func TestSetvSaveConfigError(t *testing.T) {
	kit := testKit(t)
	home := kit.d.App.Cfg.Home
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("\tnot: [valid\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	app := setvStack(kit, kit.setvEditConfigView())
	if cmd := kit.setvSaveConfig(app, map[string]any{"output": "json"}); cmd == nil {
		t.Fatal("setvSaveConfig returned nil cmd on error")
	}
	body := setvBody(app)
	if !strings.Contains(body, "parse config file") {
		t.Fatalf("expected parse error message: %q", body)
	}
}

func TestSetvAppearanceFormFields(t *testing.T) {
	kit := testKit(t)
	v := kit.setvAppearanceView()
	setvWantKeys(t, v, "theme", "keys")
	if got := len(setvFormValues(v)); got != 2 {
		t.Errorf("appearance form has %d fields, want 2", got)
	}
	if got := v.Title(); got != "appearance" {
		t.Errorf("title = %q, want appearance", got)
	}
}

func TestSetvSaveAppearancePersistsValues(t *testing.T) {
	setvKeepAppearance(t)
	kit := testKit(t)

	themeKeys, keyKeys := theme.Keys(), keys.Keys()
	if len(themeKeys) == 0 || len(keyKeys) == 0 {
		t.Fatalf("no registered themes/schemes: %v %v", themeKeys, keyKeys)
	}
	wantTheme, wantKeys := themeKeys[len(themeKeys)-1], keyKeys[len(keyKeys)-1]

	app := setvStack(kit, kit.setvAppearanceView())
	if cmd := kit.setvSaveAppearance(app, map[string]any{"theme": wantTheme, "keys": wantKeys}); cmd == nil {
		t.Fatal("setvSaveAppearance returned nil cmd")
	}
	gs := config.LoadGlobalSettings()
	if gs.Theme != wantTheme || gs.Keys != wantKeys {
		t.Fatalf("saved settings = %q/%q, want %q/%q", gs.Theme, gs.Keys, wantTheme, wantKeys)
	}
	if got := app.Top().Title(); got != "appearance" {
		t.Fatalf("Top title = %q, want appearance", got)
	}
	body := setvBody(app)
	if !strings.Contains(body, theme.DisplayName(wantTheme)) || !strings.Contains(body, keys.DisplayName(wantKeys)) {
		t.Fatalf("appearance summary missing display names: %q", body)
	}
}

func TestSetvSaveAppearanceError(t *testing.T) {
	setvKeepAppearance(t)
	kit := testKit(t)
	app := setvStack(kit, kit.setvAppearanceView())
	setvBreakConfigDir(t)

	if cmd := kit.setvSaveAppearance(app, map[string]any{"theme": "munin", "keys": "munin"}); cmd == nil {
		t.Fatal("setvSaveAppearance returned nil cmd on error")
	}
	if body := setvBody(app); !strings.Contains(body, "global settings") {
		t.Fatalf("expected global settings write error: %q", body)
	}
}

func TestSetvStatusBarFormFields(t *testing.T) {
	kit := testKit(t)
	v := kit.setvStatusBarView()
	setvWantKeys(t, v, "github", "slack", "google")
	if got := v.Title(); got != "status bar" {
		t.Errorf("title = %q, want status bar", got)
	}
	if body := setvRender(v); !strings.Contains(strings.ToUpper(body), "SHOW = VISIBLE CHIP") {
		t.Fatalf("panel caption lost: %q", body)
	}
	if got := v.Hints(); len(got) != 3 || got[1][1] != "show/hide" {
		t.Errorf("hints = %v, want show/hide legend", got)
	}
}

func TestSetvSaveStatusBarPersistsHidden(t *testing.T) {
	kit := testKit(t)
	entries := []statusBarEntry{{"github", "github"}, {"slack", "slack"}, {"google", "google"}}
	app := setvStack(kit, kit.setvStatusBarView())

	vals := map[string]any{"github": false, "slack": true, "google": false}
	if cmd := kit.setvSaveStatusBar(app, entries, vals); cmd == nil {
		t.Fatal("setvSaveStatusBar returned nil cmd")
	}
	if !config.StatusBarHidden("github") || !config.StatusBarHidden("google") {
		t.Fatalf("expected github+google hidden, got %v", config.LoadGlobalSettings().HiddenStatusBar)
	}
	if config.StatusBarHidden("slack") {
		t.Fatal("slack should stay visible")
	}
	body := setvBody(app)
	if !strings.Contains(body, "hidden: github, google") || !strings.Contains(body, "shown:  slack") {
		t.Fatalf("status bar summary wrong: %q", body)
	}
}

func TestSetvSaveStatusBarAllShown(t *testing.T) {
	kit := testKit(t)
	entries := []statusBarEntry{{"github", "github"}, {"slack", "slack"}}
	app := setvStack(kit, kit.setvStatusBarView())

	if cmd := kit.setvSaveStatusBar(app, entries, map[string]any{"github": true, "slack": true}); cmd == nil {
		t.Fatal("setvSaveStatusBar returned nil cmd")
	}
	if got := config.LoadGlobalSettings().HiddenStatusBar; len(got) != 0 {
		t.Fatalf("hidden = %v, want empty", got)
	}
	if body := setvBody(app); !strings.Contains(body, "all status chips shown") {
		t.Fatalf("expected all-shown message: %q", body)
	}
}

func TestSetvSaveStatusBarError(t *testing.T) {
	kit := testKit(t)
	app := setvStack(kit, kit.setvStatusBarView())
	setvBreakConfigDir(t)

	entries := []statusBarEntry{{"github", "github"}}
	if cmd := kit.setvSaveStatusBar(app, entries, map[string]any{"github": false}); cmd == nil {
		t.Fatal("setvSaveStatusBar returned nil cmd on error")
	}
	if body := setvBody(app); !strings.Contains(body, "global settings") {
		t.Fatalf("expected global settings write error: %q", body)
	}
}
