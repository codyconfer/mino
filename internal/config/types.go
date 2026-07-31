package config

import (
	"os"
	"strings"
)

type Config struct {
	Home     string            `koanf:"-"`
	Output   string            `koanf:"output"`
	Timeout  string            `koanf:"timeout"`
	Role     string            `koanf:"role"`
	Keybinds map[string]string `koanf:"keybinds"`
	Audit    AuditConfig       `koanf:"audit"`
	Backup   BackupConfig      `koanf:"backup"`
	GitHub   GitHubConfig      `koanf:"github"`
	Daemon   DaemonConfig      `koanf:"daemon"`
	Cache    CacheConfig       `koanf:"cache"`

	Plugins map[string]map[string]any `koanf:"plugins"`
}

func (c *Config) PluginSettings(namespace string) map[string]any {
	if c == nil || namespace == "" {
		return nil
	}
	section := c.Plugins[namespace]
	out := make(map[string]any, len(section))
	for key, value := range section {
		out[settingKey(key)] = value
	}
	overlayPluginEnv(namespace, out)
	return out
}

func settingKey(key string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(key)), ".", "_")
}

func overlayPluginEnv(namespace string, out map[string]any) {
	prefix := envPrefix + "PLUGINS_" + strings.ToUpper(settingKey(namespace)) + "_"
	for _, entry := range os.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || !strings.HasPrefix(name, prefix) {
			continue
		}
		key := settingKey(strings.TrimPrefix(name, prefix))
		if key == "" {
			continue
		}
		out[key] = value
	}
}

type CacheConfig struct {
	TTL       string            `koanf:"ttl"`
	DetailTTL string            `koanf:"detail_ttl"`
	Signals   map[string]string `koanf:"signals"`
}

type DaemonConfig struct {
	Interval string `koanf:"interval"`
	Bell     bool   `koanf:"bell"`
	Desktop  bool   `koanf:"desktop"`
	Tray     bool   `koanf:"tray"`
	Theme    string `koanf:"theme"`
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

type RoleDef struct {
	Name       string            `yaml:"name,omitempty" json:"name,omitempty"`
	Type       DirectiveType     `yaml:"type,omitempty" json:"type,omitempty"`
	Home       string            `yaml:"home" json:"home"`
	Flights    []string          `yaml:"flights" json:"flights"`
	Queries    []string          `yaml:"queries" json:"queries"`
	Formatters []string          `yaml:"formatters,omitempty" json:"formatters,omitempty"`
	Contexts   map[string]string `yaml:"contexts,omitempty" json:"contexts,omitempty"`
	Hooks      RoleHooks         `yaml:"hooks,omitempty" json:"hooks,omitempty"`
	Status     []RoleStatusBlock `yaml:"status,omitempty" json:"status,omitempty"`
}

type RoleHooks struct {
	Enter RoleShellHooks `yaml:"enter,omitempty" json:"enter,omitempty"`
	Exit  RoleShellHooks `yaml:"exit,omitempty" json:"exit,omitempty"`
}

type RoleShellHooks struct {
	Bash       string `yaml:"bash,omitempty" json:"bash,omitempty"`
	PowerShell string `yaml:"powershell,omitempty" json:"powershell,omitempty"`
}

type RoleStatusBlock struct {
	Glyph      string `yaml:"glyph,omitempty" json:"glyph,omitempty"`
	Bash       string `yaml:"bash,omitempty" json:"bash,omitempty"`
	PowerShell string `yaml:"powershell,omitempty" json:"powershell,omitempty"`
}

func (b RoleStatusBlock) Hooks() RoleShellHooks {
	return RoleShellHooks{Bash: b.Bash, PowerShell: b.PowerShell}
}

type Flight struct {
	Name      string        `yaml:"name,omitempty" json:"name,omitempty"`
	Type      DirectiveType `yaml:"type,omitempty" json:"type,omitempty"`
	Queries   []string      `yaml:"queries" json:"queries"`
	Formatter string        `yaml:"formatter,omitempty" json:"formatter,omitempty"`
}

type FormatterDef struct {
	Name     string        `yaml:"name,omitempty" json:"name,omitempty"`
	Type     DirectiveType `yaml:"type,omitempty" json:"type,omitempty"`
	Title    string        `yaml:"title,omitempty" json:"title,omitempty"`
	Template string        `yaml:"template,omitempty" json:"template,omitempty"`
}

func (f FormatterDef) Display() string {
	if f.Title != "" {
		return f.Title
	}
	return f.Name
}
