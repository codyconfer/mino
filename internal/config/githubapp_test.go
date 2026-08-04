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

func TestParseConfigAppliesGitHubServiceAuthEnvVars(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	t.Setenv("MINO_GITHUB_VIEWER", "octocat")
	t.Setenv("MINO_GITHUB_SERVICE_TOKEN", "ghp_service")
	t.Setenv("MINO_GITHUB_APP_ID", "123456")
	t.Setenv("MINO_GITHUB_APP_INSTALLATION_ID", "78901234")
	t.Setenv("MINO_GITHUB_APP_PRIVATE_KEY_PATH", "/run/secrets/app.pem")

	cfg, err := ParseConfig(t.TempDir(), nil, "yaml")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	for _, tc := range []struct{ name, got, want string }{
		{"MINO_GITHUB_VIEWER", cfg.GitHub.Viewer, "octocat"},
		{"MINO_GITHUB_SERVICE_TOKEN", cfg.GitHub.ServiceToken, "ghp_service"},
		{"MINO_GITHUB_APP_ID", cfg.GitHub.App.ID, "123456"},
		{"MINO_GITHUB_APP_INSTALLATION_ID", cfg.GitHub.App.InstallationID, "78901234"},
		{"MINO_GITHUB_APP_PRIVATE_KEY_PATH", cfg.GitHub.App.PrivateKeyPath, "/run/secrets/app.pem"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s did not apply: got %q, want %q; env is the only way to configure service auth "+
				"in a container, so a name that resolves to nothing makes the feature unreachable there",
				tc.name, tc.got, tc.want)
		}
	}
	if strings.Contains(buf.String(), "MINO_GITHUB_APP") {
		t.Errorf("warned about env vars that applied correctly:\n%s", buf.String())
	}
}

func TestGitHubAppEnvVarsResolveToTheDeepestLeaf(t *testing.T) {
	t.Setenv("MINO_GITHUB_APP_ID", "123456")
	t.Setenv("MINO_GITHUB_APP_PRIVATE_KEY_PATH", "/run/secrets/app.pem")

	cfg, err := ParseConfig(t.TempDir(), nil, "yaml")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.GitHub.App.ID != "123456" {
		t.Errorf("app.id = %q, want 123456", cfg.GitHub.App.ID)
	}
	if cfg.GitHub.App.PrivateKeyPath != "/run/secrets/app.pem" {
		t.Errorf("app.private_key_path = %q, want /run/secrets/app.pem", cfg.GitHub.App.PrivateKeyPath)
	}

	ghType := reflect.TypeFor[GitHubConfig]()
	if f, ok := ghType.FieldByName("App"); !ok || f.Tag.Get("koanf") != "app" {
		t.Fatal("GitHubConfig.App lost its `app` koanf tag; every MINO_GITHUB_APP_* var resolves through it")
	}
	for i := range ghType.NumField() {
		tag := ghType.Field(i).Tag.Get("koanf")
		if rest, ok := strings.CutPrefix(tag, "app_"); ok {
			t.Errorf("GitHubConfig has a flat %q field; the env overlay joins candidate tokens "+
				"longest-first, so it would steal MINO_GITHUB_APP_%s from github.app and the nested "+
				"setting would silently do nothing", tag, strings.ToUpper(rest))
		}
	}
}

func TestInlineAppPrivateKeyEnvVarNeverEntersConfig(t *testing.T) {
	const pem = "-----BEGIN RSA PRIVATE KEY-----\nnot-a-real-key\n-----END RSA PRIVATE KEY-----"
	t.Setenv("MINO_GITHUB_APP_PRIVATE_KEY", pem)

	cfg, err := ParseConfig(t.TempDir(), nil, "yaml")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	rendered := renderStringFields(cfg.GitHub)
	if strings.Contains(rendered, "BEGIN RSA PRIVATE KEY") || strings.Contains(rendered, "not-a-real-key") {
		t.Fatalf("MINO_GITHUB_APP_PRIVATE_KEY reached config.Config. There is no github.app.private_key "+
			"leaf on purpose: with nothing for the env overlay to bind to, key material cannot be "+
			"reflected back out of *Config, which is what keeps /api/v1/config, `mino config export` and "+
			"verify snippets safe by construction instead of by remembering to redact:\n%s", rendered)
	}
}

func TestGitHubConfigHoldsNoSecretFieldSoConfigExportsStaySafe(t *testing.T) {
	for _, tc := range []struct {
		tag        string
		wantSecret bool
	}{
		{"service_token", true},
		{"private_key", true},
		{"private_key_path", true},
		{"viewer", false},
		{"id", false},
		{"installation_id", false},
	} {
		if got := redact.IsSecretKey(tc.tag); got != tc.wantSecret {
			t.Errorf("redact.IsSecretKey(%q) = %v, want %v; every config-derived output relies on this "+
				"classification rather than on per-call-site redaction code", tc.tag, got, tc.wantSecret)
		}
	}
}

func renderStringFields(v any) string {
	var b strings.Builder
	var walk func(rv reflect.Value)
	walk = func(rv reflect.Value) {
		switch rv.Kind() {
		case reflect.Struct:
			for i := range rv.NumField() {
				walk(rv.Field(i))
			}
		case reflect.Slice, reflect.Array:
			for i := range rv.Len() {
				walk(rv.Index(i))
			}
		case reflect.String:
			b.WriteString(rv.String())
			b.WriteByte('\n')
		default:
		}
	}
	walk(reflect.ValueOf(v))
	return b.String()
}

func TestGitProviderDefaultsToEmptyAndIsEnvSettable(t *testing.T) {
	cfg, err := ParseConfig(t.TempDir(), nil, "yaml")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.GitProvider() != "" {
		t.Errorf("git.provider = %q by default, want empty so the caller falls back to gitauth.Default "+
			"and every existing install keeps using GitHub", cfg.GitProvider())
	}

	t.Setenv("MINO_GIT_PROVIDER", "gitlab")
	cfg, err = ParseConfig(t.TempDir(), nil, "yaml")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.GitProvider() != "gitlab" {
		t.Errorf("MINO_GIT_PROVIDER did not apply: %q", cfg.GitProvider())
	}
}

func TestGitSettingsRoutesGitHubKeysAndPluginNamespaces(t *testing.T) {
	cfg := Defaults()
	cfg.GitHub.APIURL = "https://ghe.example.com/api/v3"
	cfg.GitHub.ServiceToken = "ghp_x"
	cfg.GitHub.App.ID = "123"
	cfg.Plugins = map[string]map[string]any{"gitlab": {"api_url": "https://gitlab.example.com/api/v4"}}

	gh := cfg.GitSettings("github")
	for _, tc := range []struct{ key, want string }{
		{"api_url", "https://ghe.example.com/api/v3"},
		{"service_token", "ghp_x"},
		{"app.id", "123"},
		{"nonesuch", ""},
	} {
		if got := gh(tc.key); got != tc.want {
			t.Errorf("github setting %q = %q, want %q", tc.key, got, tc.want)
		}
	}

	gl := cfg.GitSettings("gitlab")
	if got := gl("api_url"); got != "https://gitlab.example.com/api/v4" {
		t.Errorf("gitlab api_url = %q; a provider from a plugin must read its settings from its own "+
			"plugins.<ns> namespace with no typed config field of its own", got)
	}
	if got := gl("service_token"); got != "" {
		t.Errorf("unset gitlab key = %q, want empty", got)
	}
}
