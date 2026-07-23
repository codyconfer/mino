package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	sconfig "github.com/codyconfer/sisyphus/config"

	"github.com/codyconfer/munin/internal/errs"
)

const (
	envHome   = "MUNIN_HOME"
	homeDir   = ".munin"
	envPrefix = "MUNIN_"
)

type Config struct {
	Home    string         `koanf:"-"`
	Output  string         `koanf:"output"`
	Timeout string         `koanf:"timeout"`
	Role    string         `koanf:"role"`
	Audit   AuditConfig    `koanf:"audit"`
	Backup  BackupConfig   `koanf:"backup"`
	GitHub  GitHubConfig   `koanf:"github"`
	Google  GoogleConfig   `koanf:"google"`
	Cal     CalendarConfig `koanf:"calendar"`
	Gmail   GmailConfig    `koanf:"gmail"`
	Docs    DocsConfig     `koanf:"docs"`
	Drive   DriveConfig    `koanf:"drive"`
	Tasks   TasksConfig    `koanf:"tasks"`
	Slack   SlackConfig    `koanf:"slack"`
	Daemon  DaemonConfig   `koanf:"daemon"`
}

type DaemonConfig struct {
	Interval string `koanf:"interval"`
	Bell     bool   `koanf:"bell"`
	Desktop  bool   `koanf:"desktop"`
	Tray     bool   `koanf:"tray"`
	Theme    string `koanf:"theme"`
}

type DriveConfig struct {
	Dir     string   `koanf:"dir"`
	Folders []string `koanf:"folders"`
	Recent  int      `koanf:"recent"`
}

type TasksConfig struct {
	List          string   `koanf:"list"`
	Lists         []string `koanf:"lists"`
	ShowCompleted bool     `koanf:"show_completed"`
	Max           int      `koanf:"max"`
}

type GoogleConfig struct {
	OAuthClientID     string `koanf:"oauth_client_id"`
	OAuthClientSecret string `koanf:"oauth_client_secret"`
}

type AuditConfig struct {
	Enabled bool   `koanf:"enabled"`
	Path    string `koanf:"path"`
}

type BackupConfig struct {
	SecretBackend string `koanf:"secret_backend"`
	SecretName    string `koanf:"secret_name"`
	Destination   string `koanf:"destination"`
	Keep          int    `koanf:"keep"`
}

type RoleDef struct {
	Name    string   `yaml:"name" json:"name"`
	Flights []string `yaml:"flights" json:"flights"`
	Queries []string `yaml:"queries" json:"queries"`
	Filters []string `yaml:"filters" json:"filters"`
}

type Flight struct {
	Name    string   `yaml:"name" json:"name"`
	Queries []string `yaml:"queries" json:"queries"`
}

type Access struct {
	Role    string
	all     bool
	flights map[string]bool
	queries map[string]bool
	filters map[string]bool
}

func NewAccess(role string, roles map[string]RoleDef) Access {
	if role == "" {
		return Access{all: true}
	}
	rd, ok := roles[role]
	if !ok {
		return Access{Role: role}
	}
	return Access{
		Role:    role,
		flights: toSet(rd.Flights),
		queries: toSet(rd.Queries),
		filters: toSet(rd.Filters),
	}
}

func (a Access) FlightVisible(name string) bool { return a.all || a.flights[name] }
func (a Access) QueryVisible(name string) bool  { return a.all || a.queries[name] }
func (a Access) FilterVisible(name string) bool { return a.all || a.filters[name] }

func toSet(xs []string) map[string]bool {
	if len(xs) == 0 {
		return nil
	}
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

type GitHubConfig struct {
	Queries       []string `koanf:"queries"`
	OAuthClientID string   `koanf:"oauth_client_id"`
	OAuthScopes   string   `koanf:"oauth_scopes"`
	APIURL        string   `koanf:"api_url"`
	Max           int      `koanf:"max"`
}

type CalendarConfig struct {
	CalendarID string `koanf:"calendar_id"`
	Window     string `koanf:"window"`
	Max        int    `koanf:"max"`
}

type GmailConfig struct {
	Query string `koanf:"query"`
	Max   int    `koanf:"max"`
}

type DocsConfig struct {
	Recent int `koanf:"recent"`
}

type SlackConfig struct {
	TokenEnv          string `koanf:"token_env"`
	AppTokenEnv       string `koanf:"app_token_env"`
	BotTokenEnv       string `koanf:"bot_token_env"`
	OAuthClientID     string `koanf:"oauth_client_id"`
	OAuthClientSecret string `koanf:"oauth_client_secret"`
	UserScopes        string `koanf:"user_scopes"`
	Limit             int    `koanf:"limit"`
}

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

type GlobalSettings struct {
	Home            string `yaml:"home"`
	Theme           string `yaml:"theme"`
	Keys            string `yaml:"keys"`
	PreferDB        bool   `yaml:"prefer_duckdb"`
	LogLevel        string `yaml:"log_level"`
	LogColor        string `yaml:"log_color"`
	Onboarded       bool   `yaml:"onboarded"`
	OnboardedDomain string `yaml:"onboarded_domain"`
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
