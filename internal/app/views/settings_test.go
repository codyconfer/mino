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
