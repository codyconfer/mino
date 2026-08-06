package config

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/codyconfer/sisyphus/redact"

	"github.com/codyconfer/mino/internal/log"
)

func TestParseConfigReadsTheGiteaSection(t *testing.T) {
	raw := []byte(`
git:
  provider: gitea
gitea:
  url: https://git.example.com
  viewer: alice
  max: 10
  queries:
    - "type:pulls state:open created:@me"
`)
	cfg, err := ParseConfig(t.TempDir(), raw, "yaml")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.GitProvider() != "gitea" {
		t.Errorf("GitProvider() = %q, want gitea", cfg.GitProvider())
	}
	if cfg.Gitea.URL != "https://git.example.com" || cfg.Gitea.Viewer != "alice" || cfg.Gitea.Max != 10 {
		t.Errorf("gitea section = %+v, want the file's values", cfg.Gitea)
	}
	if want := []string{"type:pulls state:open created:@me"}; !reflect.DeepEqual(cfg.Gitea.Queries, want) {
		t.Errorf("queries = %v, want %v", cfg.Gitea.Queries, want)
	}
}

func TestGiteaMaxDefaults(t *testing.T) {
	cfg, err := ParseConfig(t.TempDir(), nil, "yaml")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Gitea.Max != 30 {
		t.Errorf("gitea.max = %d, want 30", cfg.Gitea.Max)
	}
}

func TestParseConfigAppliesGiteaEnvVars(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	t.Setenv("MINO_GITEA_URL", "https://git.example.com")
	t.Setenv("MINO_GITEA_API_URL", "https://git.example.com/api/v1")
	t.Setenv("MINO_GITEA_SERVICE_TOKEN", "gta_service")
	t.Setenv("MINO_GITEA_VIEWER", "acme-bot")

	cfg, err := ParseConfig(t.TempDir(), nil, "yaml")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	for _, tc := range []struct{ name, got, want string }{
		{"MINO_GITEA_URL", cfg.Gitea.URL, "https://git.example.com"},
		{"MINO_GITEA_API_URL", cfg.Gitea.APIURL, "https://git.example.com/api/v1"},
		{"MINO_GITEA_SERVICE_TOKEN", cfg.Gitea.ServiceToken, "gta_service"},
		{"MINO_GITEA_VIEWER", cfg.Gitea.Viewer, "acme-bot"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s did not apply: got %q, want %q; env is the only way to configure a self-hosted "+
				"forge in a container", tc.name, tc.got, tc.want)
		}
	}
	if strings.Contains(buf.String(), "MINO_GITEA") {
		t.Errorf("warned about env vars that applied correctly:\n%s", buf.String())
	}
}

func TestGiteaSectionDoesNotStealGitProvider(t *testing.T) {
	t.Setenv("MINO_GIT_PROVIDER", "gitea")
	t.Setenv("MINO_GITEA_URL", "https://git.example.com")

	cfg, err := ParseConfig(t.TempDir(), nil, "yaml")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.GitProvider() != "gitea" {
		t.Errorf("GitProvider() = %q, want gitea; git and gitea are sibling sections and the longer "+
			"prefix must not swallow MINO_GIT_PROVIDER", cfg.GitProvider())
	}
	if cfg.Gitea.URL != "https://git.example.com" {
		t.Errorf("gitea.url = %q, want the env value; the shorter git prefix must not swallow it either", cfg.Gitea.URL)
	}
}

func TestNoGiteaTokenConfigKeyExists(t *testing.T) {
	t.Setenv("MINO_GITEA_TOKEN", "gta_personal")

	cfg, err := ParseConfig(t.TempDir(), nil, "yaml")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if strings.Contains(dumpConfig(t, cfg), "gta_personal") {
		t.Error("a personal access token reached the loaded config; with no field to bind to it cannot be " +
			"reflected back out through /api/v1/config or `mino config export`")
	}
}

func TestGiteaServiceTokenIsTreatedAsSecret(t *testing.T) {
	if !redact.IsSecretKey("service_token") {
		t.Error("service_token is not treated as secret, so gitea.service_token would survive a config export")
	}
}

func TestGitSettingsRoutesGiteaAndForgejoToTheTypedSection(t *testing.T) {
	cfg := Defaults()
	cfg.Gitea = GiteaConfig{
		URL:          "https://git.example.com",
		APIURL:       "https://git.example.com/api/v1",
		ServiceToken: "gta_service",
		Viewer:       "alice",
	}

	for _, provider := range []string{"gitea", "forgejo"} {
		t.Run(provider, func(t *testing.T) {
			get := cfg.GitSettings(provider)
			for _, tc := range []struct{ key, want string }{
				{"url", "https://git.example.com"},
				{"api_url", "https://git.example.com/api/v1"},
				{"service_token", "gta_service"},
				{"viewer", "alice"},
				{"app.id", ""},
			} {
				if got := get(tc.key); got != tc.want {
					t.Errorf("GitSettings(%q)(%q) = %q, want %q; both names read one section", provider, tc.key, got, tc.want)
				}
			}
		})
	}
}

func dumpConfig(t *testing.T, cfg *Config) string {
	t.Helper()
	return strings.Join([]string{
		cfg.Gitea.URL, cfg.Gitea.APIURL, cfg.Gitea.ServiceToken, cfg.Gitea.Viewer,
		strings.Join(cfg.Gitea.Queries, " "),
	}, "\x00")
}
