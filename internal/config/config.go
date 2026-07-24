package config

import (
	"os"

	sconfig "github.com/codyconfer/sisyphus/config"

	"github.com/codyconfer/munin/internal/errs"
)

const (
	envHome   = "MUNIN_HOME"
	envLogDir = "MUNIN_LOG_DIR"
	homeDir   = ".munin"
	envPrefix = "MUNIN_"
)

func Home(override string) (string, error) {
	if override == "" {
		if h := os.Getenv(envHome); h != "" {
			override = h
		} else if h := globalHome(); h != "" {
			override = h
		}
	}
	home, err := sconfig.Home(override, "", homeDir)
	if err != nil {
		return "", errs.Wrap(errs.KindConfig, err, "resolve home directory")
	}
	return home, nil
}

func Defaults() *Config {
	return &Config{
		Output:  "terminal",
		Timeout: "30s",
		Audit:   AuditConfig{Enabled: true},
		Backup:  BackupConfig{SecretBackend: "auto", SecretName: "munin-backup-key", Destination: "local"},
		GitHub:  GitHubConfig{OAuthScopes: "repo read:org", Max: 30},
		Cal:     CalendarConfig{CalendarID: "primary", Window: "24h", Max: 50},
		Gmail:   GmailConfig{Query: "is:unread in:inbox", Max: 15},
		Docs:    DocsConfig{Recent: 10},
		Drive:   DriveConfig{Recent: 20},
		Tasks:   TasksConfig{Max: 100},
		Slack:   SlackConfig{TokenEnv: "SLACK_TOKEN", AppTokenEnv: "SLACK_APP_TOKEN", BotTokenEnv: "SLACK_BOT_TOKEN", Limit: 50},
		Daemon:  DaemonConfig{Interval: "60s", Bell: true, Theme: "dark"},
	}
}

func ReadConfigFile(homeOverride string) (home string, raw []byte, format string, err error) {
	home, err = Home(homeOverride)
	if err != nil {
		return "", nil, "", err
	}
	raw, format, err = sconfig.ReadFile(home)
	if err != nil {
		return home, nil, "", errs.Wrapf(errs.KindConfig, err, "read config file").WithHint("checked under %s", home)
	}
	return home, raw, format, nil
}

func ParseConfig(home string, raw []byte, format string) (*Config, error) {
	cfg := Defaults()
	cfg.Home = home
	if err := sconfig.ParseInto(cfg, raw, format, envPrefix); err != nil {
		return nil, errs.Wrapf(errs.KindConfig, err, "parse config")
	}
	cfg.Home = home
	return cfg, nil
}

func Load(homeOverride string) (*Config, error) {
	home, raw, format, err := ReadConfigFile(homeOverride)
	if err != nil {
		return nil, err
	}
	return ParseConfig(home, raw, format)
}
