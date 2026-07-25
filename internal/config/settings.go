package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	sconfig "github.com/codyconfer/sisyphus/config"

	"github.com/codyconfer/munin/internal/errs"
)

// GlobalSettings is ~/.config/munin/settings.yaml (theme/keys/onboarding, not per-home).
type GlobalSettings struct {
	Home            string `yaml:"home"`
	Theme           string `yaml:"theme"`
	Keys            string `yaml:"keys"`
	PreferDB        bool   `yaml:"prefer_duckdb"`
	LogLevel        string `yaml:"log_level"`
	LogColor        string `yaml:"log_color"`
	LogDir          string `yaml:"log_dir"`
	Onboarded       bool   `yaml:"onboarded"`
	OnboardedDomain string `yaml:"onboarded_domain"`
	// InstalledPlugins lists compile-time plugin ids in the managed set
	// (shown in the Plugins TUI). Distinct from DisabledPlugins: disable keeps
	// the id here; uninstall removes it.
	InstalledPlugins []string `yaml:"installed_plugins,omitempty"`
	// DisabledPlugins lists compile-time plugin ids that are runtime-disabled.
	// A disabled plugin may still be installed (listed); uninstall clears both.
	DisabledPlugins []string `yaml:"disabled_plugins,omitempty"`
	// HiddenStatusBar lists status-bar chip ids the user chose to hide.
	// Display-only: does not disable or uninstall plugins.
	HiddenStatusBar []string `yaml:"hidden_status_bar,omitempty"`
}

func LogDir(home string) string {
	if d := os.Getenv(envLogDir); d != "" {
		return d
	}
	if d := LoadGlobalSettings().LogDir; d != "" {
		return d
	}
	return filepath.Join(home, DirLogs)
}

func GlobalSettingsPath() string {
	path, err := sconfig.UserConfigPath("munin", "settings.yaml")
	if err != nil {
		return ""
	}
	return path
}

func LoadGlobalSettings() GlobalSettings {
	var gs GlobalSettings
	path := GlobalSettingsPath()
	if path == "" {
		return gs
	}
	data, ok, err := sconfig.ReadRaw(path)
	if err != nil || !ok {
		return gs
	}
	_ = yaml.Unmarshal(data, &gs)
	return gs
}

func SaveGlobalSettings(gs GlobalSettings) error {
	path := GlobalSettingsPath()
	if path == "" {
		return errs.New(errs.KindInternal, "cannot resolve global settings path")
	}
	data, err := yaml.Marshal(gs)
	if err != nil {
		return errs.Wrap(errs.KindInternal, err, "marshal global settings")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return errs.Wrap(errs.KindInternal, err, "create global settings dir")
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return errs.Wrap(errs.KindInternal, err, "write global settings")
	}
	return nil
}

func globalHome() string { return LoadGlobalSettings().Home }

// legacyGoogleStatusBarIDs are pre-collapse per-signal hide keys. They map to
// the single "google" status-bar chip.
var legacyGoogleStatusBarIDs = []string{"calendar", "gmail", "docs", "drive", "tasks"}

func isLegacyGoogleStatusBarID(id string) bool {
	for _, g := range legacyGoogleStatusBarIDs {
		if g == id {
			return true
		}
	}
	return false
}

// StatusBarHidden reports whether id is in the display-only hide list.
// The collapsed "google" chip is hidden when "google" or any legacy Google
// signal key (calendar/gmail/docs/drive/tasks) is listed.
func StatusBarHidden(id string) bool {
	if id == "" {
		return false
	}
	hidden := LoadGlobalSettings().HiddenStatusBar
	if id == "google" {
		for _, h := range hidden {
			if h == "google" || isLegacyGoogleStatusBarID(h) {
				return true
			}
		}
		return false
	}
	for _, h := range hidden {
		if h == id {
			return true
		}
	}
	return false
}

// SetHiddenStatusBar replaces the status-bar hide list and persists settings.
func SetHiddenStatusBar(ids []string) error {
	gs := LoadGlobalSettings()
	gs.HiddenStatusBar = normalizeHiddenStatusBar(ids)
	return SaveGlobalSettings(gs)
}

func normalizeHiddenStatusBar(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	googleHidden := false
	for _, id := range ids {
		if id == "" {
			continue
		}
		if id == "google" || isLegacyGoogleStatusBarID(id) {
			googleHidden = true
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if googleHidden {
		if _, ok := seen["google"]; !ok {
			out = append(out, "google")
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
