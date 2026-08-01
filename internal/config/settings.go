package config

import (
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	sconfig "github.com/codyconfer/sisyphus/config"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
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
	path, err := sconfig.UserConfigPath("mino", "settings.yaml")
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

type settingsEntry struct {
	gs   GlobalSettings
	mod  time.Time
	size int64
}

var (
	settingsCacheMu sync.Mutex
	settingsCache   = map[string]settingsEntry{}
)

// clone detaches the slice fields so cached settings are never aliased.
func (gs GlobalSettings) clone() GlobalSettings {
	gs.InstalledPlugins = slices.Clone(gs.InstalledPlugins)
	gs.DisabledPlugins = slices.Clone(gs.DisabledPlugins)
	gs.HiddenStatusBar = slices.Clone(gs.HiddenStatusBar)
	return gs
}

func dropCachedSettings(path string) {
	settingsCacheMu.Lock()
	delete(settingsCache, path)
	settingsCacheMu.Unlock()
}

// cachedGlobalSettingsAt reuses the last parse of path while its mtime and size
// are unchanged; misses, missing files and errors always hit the disk.
func cachedGlobalSettingsAt(path string) (GlobalSettings, error) {
	fi, statErr := os.Stat(path)
	if statErr != nil || !fi.Mode().IsRegular() {
		dropCachedSettings(path)
		return readGlobalSettingsAt(path)
	}
	settingsCacheMu.Lock()
	e, ok := settingsCache[path]
	settingsCacheMu.Unlock()
	if ok && e.size == fi.Size() && e.mod.Equal(fi.ModTime()) {
		return e.gs.clone(), nil
	}
	gs, err := readGlobalSettingsAt(path)
	if err != nil {
		dropCachedSettings(path)
		return gs, err
	}
	settingsCacheMu.Lock()
	settingsCache[path] = settingsEntry{gs: gs.clone(), mod: fi.ModTime(), size: fi.Size()}
	settingsCacheMu.Unlock()
	return gs, nil
}

func loadGlobalSettings() (string, GlobalSettings, error) {
	path := GlobalSettingsPath()
	if path == "" {
		return "", GlobalSettings{}, errs.New(errs.KindInternal, "cannot resolve global settings path")
	}
	gs, err := cachedGlobalSettingsAt(path)
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
		log.Warnf("%s: falling back to built-in defaults; mino will not overwrite the file", msg)
	}
	return GlobalSettings{}
}

func SaveGlobalSettings(gs GlobalSettings) error {
	path := GlobalSettingsPath()
	if path == "" {
		return errs.New(errs.KindInternal, "cannot resolve global settings path")
	}
	defer dropCachedSettings(path)
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

// HiddenStatusBar returns the user's hidden status-bar ids, for callers that
// test many ids at once via StatusBarHiddenIn.
func HiddenStatusBar() []string { return LoadGlobalSettings().HiddenStatusBar }

func StatusBarHidden(id string) bool {
	return StatusBarHiddenIn(HiddenStatusBar(), id)
}

// StatusBarHiddenIn reports whether id is hidden per an already-read hidden list.
func StatusBarHiddenIn(hidden []string, id string) bool {
	if id == "" {
		return false
	}
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
