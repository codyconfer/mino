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
	// DisabledPlugins lists compile-time plugin ids that are runtime-disabled (ADR-13).
	DisabledPlugins []string `yaml:"disabled_plugins,omitempty"`
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
