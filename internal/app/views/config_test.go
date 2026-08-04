package views

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func configFor(t *testing.T, kit *Kit) *configView {
	t.Helper()
	v, ok := kit.Config().(*configView)
	if !ok {
		t.Fatal("config view has the wrong type")
	}
	return v
}

func configValues(t *testing.T, kit *Kit) map[string]any {
	t.Helper()
	return configFor(t, kit).Form().Values()
}

func (v *configView) set(t *testing.T, key, val string) {
	t.Helper()
	for i := range v.Form().Fields {
		if v.Form().Fields[i].Key == key {
			v.Form().Fields[i].Text = val
			return
		}
	}
	t.Fatalf("config form has no field %q", key)
}

func (v *configView) pick(t *testing.T, key, val string) {
	t.Helper()
	for i := range v.Form().Fields {
		fd := &v.Form().Fields[i]
		if fd.Key != key {
			continue
		}
		for j, opt := range fd.Options {
			if opt == val {
				fd.Selected = j
				return
			}
		}
		t.Fatalf("config field %q has no option %q: %v", key, val, fd.Options)
	}
	t.Fatalf("config form has no field %q", key)
}

func TestConfigEditorSeedsFieldsFromTheLiveConfig(t *testing.T) {
	kit := testKit(t)
	kit.d.App.Cfg.Output = "json"
	kit.d.App.Cfg.Timeout = "45s"
	kit.d.App.Cfg.Backup.Keep = 7

	vals := configValues(t, kit)
	for _, key := range []string{"output", "audit.enabled", "timeout", "cache.ttl", "backup.destination", "backup.keep"} {
		if _, ok := vals[key]; !ok {
			t.Errorf("config editor missing field %q: %v", key, vals)
		}
	}
	if vals["output"] != "json" {
		t.Errorf("output = %v, want json", vals["output"])
	}
	if vals["timeout"] != "45s" {
		t.Errorf("timeout = %v, want 45s", vals["timeout"])
	}
	if vals["backup.keep"] != "7" {
		t.Errorf("backup.keep = %v, want 7", vals["backup.keep"])
	}
	if got := configFor(t, kit).Title(); got != "config" {
		t.Errorf("title = %q, want config", got)
	}
}

func TestConfigEditorSaveWritesConfigFile(t *testing.T) {
	kit := testKit(t)
	home := kit.d.App.Cfg.Home
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("output: terminal\nrole: triage\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	v := configFor(t, kit)
	v.pick(t, "output", "json")
	v.set(t, "timeout", "45s")
	v.set(t, "backup.keep", "7")

	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	step(app, tea.KeyMsg{Type: tea.KeyCtrlS})

	raw, err := os.ReadFile(filepath.Join(home, "config.yaml"))
	if err != nil {
		t.Fatalf("save did not write the config: %v (status %q)", err, v.Status())
	}
	body := string(raw)
	for _, want := range []string{"output: json", "timeout: 45s", "keep: 7", "role: triage"} {
		if !strings.Contains(body, want) {
			t.Errorf("saved config missing %q:\n%s", want, body)
		}
	}
}

func TestConfigEditorSaveCreatesConfigFile(t *testing.T) {
	kit := testKit(t)
	home := kit.d.App.Cfg.Home

	v := configFor(t, kit)
	if name := v.editorSavedName(); name != "" {
		t.Fatalf("saved name = %q, want empty without a config file", name)
	}
	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	step(app, tea.KeyMsg{Type: tea.KeyCtrlS})

	if _, err := os.Stat(filepath.Join(home, "config.yaml")); err != nil {
		t.Fatalf("save did not create config.yaml: %v (status %q)", err, v.Status())
	}
	if name := v.editorSavedName(); name != "config.yaml" {
		t.Errorf("saved name = %q, want config.yaml", name)
	}
}

func TestConfigEditorSaveReportsParseErrors(t *testing.T) {
	kit := testKit(t)
	if err := os.WriteFile(filepath.Join(kit.d.App.Cfg.Home, "config.yaml"), []byte("\tnot: [valid\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	v := configFor(t, kit)
	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	step(app, tea.KeyMsg{Type: tea.KeyCtrlS})

	if !strings.Contains(v.Status(), "parse config file") {
		t.Fatalf("status = %q, want a parse error", v.Status())
	}
}

func TestConfigEditorYAMLPreviewNestsDottedKeys(t *testing.T) {
	kit := testKit(t)
	v := configFor(t, kit)
	v.set(t, "backup.keep", "3")

	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	if strings.Contains(app.View(), "keep: 3") {
		t.Fatal("yaml preview showing before it was toggled on")
	}
	app = step(app, tea.KeyMsg{Type: tea.KeyCtrlY})
	body := app.View()
	for _, want := range []string{"backup:", "keep: 3", "daemon:"} {
		if !strings.Contains(body, want) {
			t.Errorf("yaml preview missing %q: %q", want, body)
		}
	}
	if strings.Contains(body, "backup.keep") {
		t.Errorf("yaml preview still shows dotted keys: %q", body)
	}
}

func TestConfigEditorValidateListsEveryFinding(t *testing.T) {
	kit := testKit(t)
	v := configFor(t, kit)
	v.set(t, "cache.ttl", "9x")

	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	step(app, tea.KeyMsg{Type: tea.KeyCtrlT})

	notice := v.Notice()
	if len(notice) < 2 {
		t.Fatalf("validate should list every config check, got %v", notice)
	}
	joined := strings.Join(notice, "\n")
	for _, want := range []string{"output", "cache.ttl", "is not a valid duration"} {
		if !strings.Contains(joined, want) {
			t.Errorf("validate output missing %q:\n%s", want, joined)
		}
	}
}

func TestConfigEditorRunShowsFindingsInResults(t *testing.T) {
	kit := testKit(t)
	v := configFor(t, kit)
	v.set(t, "timeout", "nope")

	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	app, cmd := update(app, tea.KeyMsg{Type: tea.KeyCtrlR})
	for _, c := range flattenCmds(cmd) {
		if c != nil {
			app = step(app, c())
		}
	}
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})

	body := app.View()
	for _, want := range []string{"config", "timeout"} {
		if !strings.Contains(body, want) {
			t.Fatalf("results panel missing %q: %q", want, body)
		}
	}
}

func TestConfigEditorDeleteRemovesTheConfigFile(t *testing.T) {
	kit := testKit(t)
	path := filepath.Join(kit.d.App.Cfg.Home, "config.yaml")

	v := configFor(t, kit)
	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	step(app, tea.KeyMsg{Type: tea.KeyCtrlX})
	if v.Confirming() {
		t.Fatal("delete should be refused without a config file")
	}
	if !strings.Contains(v.Status(), "nothing to delete") {
		t.Errorf("status = %q, want nothing-to-delete", v.Status())
	}

	if err := os.WriteFile(path, []byte("output: terminal\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	v = configFor(t, kit)
	app = newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	app = step(app, tea.KeyMsg{Type: tea.KeyCtrlX})
	if !v.Confirming() {
		t.Fatal("delete did not raise a confirmation")
	}
	app = step(app, tea.KeyMsg{Type: tea.KeyLeft})
	app, cmd := update(app, tea.KeyMsg{Type: tea.KeyEnter})
	for _, c := range flattenCmds(cmd) {
		if c != nil {
			app = step(app, c())
		}
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("confirming delete left the config file in place")
	}
}
