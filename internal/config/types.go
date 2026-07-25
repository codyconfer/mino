package config

// Config is the parsed ~/.munin config.yaml surface.
type Config struct {
	Home    string `koanf:"-"`
	Output  string `koanf:"output"`
	Timeout string `koanf:"timeout"`
	Role    string `koanf:"role"`
	// Keybinds maps bubbletea key strings (e.g. "alt+n") to a target:
	//   ntr.note.new | ntr.task.new | ntr.remind.new  — open NTR create forms
	//   <flight-name> or flight:<name>                 — open that flight in the TUI
	Keybinds map[string]string `koanf:"keybinds"`
	Audit    AuditConfig       `koanf:"audit"`
	Backup   BackupConfig      `koanf:"backup"`
	GitHub   GitHubConfig      `koanf:"github"`
	Google   GoogleConfig      `koanf:"google"`
	Cal      CalendarConfig    `koanf:"calendar"`
	Gmail    GmailConfig       `koanf:"gmail"`
	Docs     DocsConfig        `koanf:"docs"`
	Drive    DriveConfig       `koanf:"drive"`
	Tasks    TasksConfig       `koanf:"tasks"`
	Slack    SlackConfig       `koanf:"slack"`
	Daemon   DaemonConfig      `koanf:"daemon"`
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

type RoleDef struct {
	Name    string   `yaml:"name" json:"name"`
	Home    string   `yaml:"home" json:"home"`
	Flights []string `yaml:"flights" json:"flights"`
	Queries []string `yaml:"queries" json:"queries"`
	Filters []string `yaml:"filters" json:"filters"`
	// Contexts maps tool → context name applied on role activation.
	Contexts map[string]string `yaml:"contexts,omitempty" json:"contexts,omitempty"`
}

// Flight is one-per-file under flights/.
type Flight struct {
	Name    string   `yaml:"name" json:"name"`
	Queries []string `yaml:"queries" json:"queries"`
}
