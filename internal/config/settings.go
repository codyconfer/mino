package config

import (
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"

	sconfig "github.com/codyconfer/sisyphus/config"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/log"
)

type GlobalSettings struct {
	Home             string   `yaml:"home"`
	Theme            string   `yaml:"theme"`
	Keys             string   `yaml:"keys"`
	PreferDB         bool     `yaml:"prefer_duckdb"`
	LogLevel         string   `yaml:"log_level"`
	LogColor         string   `yaml:"log_color"`
	LogDir           string   `yaml:"log_dir"`
	DetailCacheTTL   string   `yaml:"detail_cache_ttl,omitempty"`
	Onboarded        bool     `yaml:"onboarded"`
	OnboardedDomain  string   `yaml:"onboarded_domain"`
	InstalledPlugins []string `yaml:"installed_plugins,omitempty"`
	DisabledPlugins  []string `yaml:"disabled_plugins,omitempty"`
	HiddenStatusBar  []string `yaml:"hidden_status_bar,omitempty"`
}

const DefaultDetailTTL = "5m"

func ResolveDetailTTL(local, global string) string {
	for _, v := range []string{local, global, DefaultDetailTTL} {
		if v != "" {
			return v
		}
	}
	return DefaultDetailTTL
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

var (
	settingsWarnMu sync.Mutex
	settingsWarned = map[string]string{}
)

func readGlobalSettingsAt(path string) (GlobalSettings, error) {
	var gs GlobalSettings
	data, ok, err := sconfig.ReadRaw(path)
	if err != nil {
		return gs, errs.Wrap(errs.KindConfig, err, "read global settings")
	}
	if !ok {
		return gs, nil
	}
	if err := yaml.Unmarshal(data, &gs); err != nil {
		return GlobalSettings{}, errs.Wrapf(errs.KindConfig, err, "malformed global settings %s", path)
	}
	return gs, nil
}

func loadGlobalSettings() (string, GlobalSettings, error) {
	path := GlobalSettingsPath()
	if path == "" {
		return "", GlobalSettings{}, errs.New(errs.KindInternal, "cannot resolve global settings path")
	}
	gs, err := readGlobalSettingsAt(path)
	return path, gs, err
}

func ReadGlobalSettings() (GlobalSettings, error) {
	_, gs, err := loadGlobalSettings()
	return gs, err
}

func LoadGlobalSettings() GlobalSettings {
	path, gs, err := loadGlobalSettings()
	settingsWarnMu.Lock()
	defer settingsWarnMu.Unlock()
	if err == nil {
		delete(settingsWarned, path)
		return gs
	}
	msg := err.Error()
	if settingsWarned[path] != msg {
		settingsWarned[path] = msg
		log.Warnf("%s: falling back to built-in defaults; munin will not overwrite the file", msg)
	}
	return GlobalSettings{}
}

func SaveGlobalSettings(gs GlobalSettings) error {
	path := GlobalSettingsPath()
	if path == "" {
		return errs.New(errs.KindInternal, "cannot resolve global settings path")
	}
	if _, err := readGlobalSettingsAt(path); err != nil {
		return errs.Wrap(errs.KindConfig, err, "refusing to overwrite global settings").
			WithHint("fix the syntax in %s, or delete the file to start from defaults", path)
	}
	data, err := yaml.Marshal(gs)
	if err != nil {
		return errs.Wrap(errs.KindInternal, err, "marshal global settings")
	}
	if _, err := sconfig.WriteItem(filepath.Dir(path), filepath.Base(path), data); err != nil {
		return errs.Wrap(errs.KindInternal, err, "write global settings")
	}
	return nil
}

func globalHome() string { return LoadGlobalSettings().Home }

var legacyGoogleStatusBarIDs = []string{"calendar", "gmail", "docs", "drive", "tasks"}

func isLegacyGoogleStatusBarID(id string) bool {
	for _, g := range legacyGoogleStatusBarIDs {
		if g == id {
			return true
		}
	}
	return false
}

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
