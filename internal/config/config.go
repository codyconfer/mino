package config

import (
	"os"
	"path/filepath"
	"strings"

	sconfig "github.com/codyconfer/sisyphus/config"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/log"
)

const (
	envHome     = "MUNIN_HOME"
	envLogDir   = "MUNIN_LOG_DIR"
	HomeDirName = ".munin"
	envPrefix   = "MUNIN_"
	homeDir     = HomeDirName
)

func DefaultHome() (string, error) {
	home, err := sconfig.Home("", "", HomeDirName)
	if err != nil {
		return "", errs.Wrap(errs.KindConfig, err, "resolve default home directory")
	}
	abs, err := filepath.Abs(home)
	if err != nil {
		return "", errs.Wrap(errs.KindConfig, err, "resolve default home directory")
	}
	return abs, nil
}

func Home(override string) (string, error) {
	if override == "" {
		if h := os.Getenv(envHome); h != "" {
			override = h
		} else if h := globalHome(); h != "" {
			override = h
		}
	}
	if override != "" {
		expanded, err := expandHomePath(override)
		if err != nil {
			return "", errs.Wrap(errs.KindConfig, err, "resolve home directory")
		}
		override = expanded
	}
	home, err := sconfig.Home(override, "", HomeDirName)
	if err != nil {
		return "", errs.Wrap(errs.KindConfig, err, "resolve home directory")
	}
	abs, err := filepath.Abs(home)
	if err != nil {
		return "", errs.Wrap(errs.KindConfig, err, "resolve home directory")
	}
	return abs, nil
}

func expandHomePath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", nil
	}
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		hd, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if p == "~" {
			return hd, nil
		}
		return filepath.Join(hd, p[2:]), nil
	}
	return p, nil
}

func DefaultKeybinds() map[string]string {
	return map[string]string{
		"alt+n": "ntr.note.new",
		"alt+r": "ntr.remind.new",
		"alt+t": "ntr.task.new",
		"alt+[": "role.prev",
		"alt+]": "role.next",
	}
}

func Defaults() *Config {
	return &Config{
		Output:   "terminal",
		Timeout:  "30s",
		Keybinds: DefaultKeybinds(),
		Audit:    AuditConfig{Enabled: true},
		Backup:   BackupConfig{SecretBackend: "auto", SecretName: "munin-backup-key", Destination: "local"},
		GitHub:   GitHubConfig{OAuthScopes: "repo read:org", Max: 30},
		Daemon:   DaemonConfig{Interval: "60s", Bell: true, Theme: "dark"},
		Cache:    CacheConfig{TTL: "60s"},
	}
}

func ConfigBasenames() []string { return []string{"config.yaml", "config.yml", "config.json"} }

func formatOfBasename(name string) string {
	if strings.EqualFold(filepath.Ext(name), ".json") {
		return "json"
	}
	return "yaml"
}

func readConfigFileNamed(homeOverride string) (home, name string, raw []byte, format string, err error) {
	home, err = Home(homeOverride)
	if err != nil {
		return "", "", nil, "", err
	}
	for _, base := range ConfigBasenames() {
		data, ok, rerr := sconfig.ReadRaw(filepath.Join(home, base))
		if rerr != nil {
			return home, "", nil, "", errs.Wrap(errs.KindConfig, rerr, "read config file").
				WithHint("checked under %s", home)
		}
		if ok {
			return home, base, data, formatOfBasename(base), nil
		}
	}
	return home, "", nil, "", nil
}

func ReadConfigFile(homeOverride string) (home string, raw []byte, format string, err error) {
	home, _, raw, format, err = readConfigFileNamed(homeOverride)
	return home, raw, format, err
}

func ConfigFilePath(homeOverride string) (string, error) {
	home, err := Home(homeOverride)
	if err != nil {
		return "", err
	}
	for _, name := range ConfigBasenames() {
		path := filepath.Join(home, name)
		if sconfig.IsFile(path) {
			return path, nil
		}
	}
	return "", errs.Newf(errs.KindConfig, "no config file found in %s", home).
		WithHint("expected config.yaml, config.yml, or config.json; create one from Settings or run `munin install`")
}

func ParseConfig(home string, raw []byte, format string) (*Config, error) {
	cfg := Defaults()
	cfg.Home = home
	if err := sconfig.ParseInto(cfg, raw, format, envPrefix, sconfig.WithEnvSectionWarning(warnEnvSection)); err != nil {
		return nil, errs.Wrapf(errs.KindConfig, err, "parse config")
	}
	cfg.Home = home
	return cfg, nil
}

func warnEnvSection(name string, section []string) {
	path := strings.Join(section, ".")
	log.Warnf("config: %s is ignored: %s is a config section, not a single value", name, path)
	log.Warnf("config: set a leaf instead, e.g. %s_<KEY> for a key under %s", name, path)
}

func Load(homeOverride string) (*Config, error) {
	home, raw, format, err := ReadConfigFile(homeOverride)
	if err != nil {
		return nil, err
	}
	return ParseConfig(home, raw, format)
}
