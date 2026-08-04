package views

import (
	"strings"
	"testing"

	"github.com/codyconfer/viewkit/forms"

	"github.com/codyconfer/mino/internal/config"
)

func TestConfigEditorHasCoreServeFields(t *testing.T) {
	kit := testKit(t)
	vals := configValues(t, kit)
	for _, key := range []string{"daemon.interval", "daemon.bell", "daemon.desktop", "daemon.theme"} {
		if _, ok := vals[key]; !ok {
			t.Errorf("config editor missing serve field %q: %v", key, vals)
		}
	}
	if _, ok := vals["daemon.tray"]; ok {
		t.Error("daemon.tray belongs to the optional daemon package, not core views")
	}
	if body := setvRender(kit.Config()); !strings.Contains(body, "daemon.interval") {
		t.Fatalf("config editor body missing daemon rows: %q", body)
	}
}

func TestSettingsStatusBarHasNoDaemonChipWithoutRegistration(t *testing.T) {
	kit := testKit(t)
	if _, ok := setvFormValues(kit.setvStatusBarView())["daemon"]; ok {
		t.Error("daemon status chip must come from a registered section")
	}
	body := setvRender(kit.setvStatusBarView())
	for _, want := range []string{"github", "slack", "google"} {
		if !strings.Contains(body, want) {
			t.Fatalf("status bar lost builtin %q: %q", want, body)
		}
	}
}

func TestRegisteredSectionReachesFormAndStatusBar(t *testing.T) {
	t.Cleanup(func() { settingsSections = nil })
	RegisterSettingsSection(SettingsSection{
		Fields: func(*config.Config) []forms.Field {
			return []forms.Field{{Key: "ext.toggle", Label: "ext.toggle", Kind: forms.FieldToggle, On: true}}
		},
		Values: func(vals, set map[string]any) {
			set["ext.toggle"] = forms.Bool(vals, "ext.toggle")
		},
		StatusBar: []StatusBarEntry{{ID: "ext", Label: "ext"}},
	})

	kit := testKit(t)
	if _, ok := configValues(t, kit)["ext.toggle"]; !ok {
		t.Error("registered section field missing from the config editor")
	}
	if _, ok := setvFormValues(kit.setvStatusBarView())["ext"]; !ok {
		t.Error("registered section status-bar entry missing")
	}

	set := map[string]any{}
	sectionValues(map[string]any{"ext.toggle": true}, set)
	if set["ext.toggle"] != true {
		t.Errorf("registered section Values not applied: %v", set)
	}
}
