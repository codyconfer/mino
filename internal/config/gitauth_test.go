package config

import "testing"

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

func TestGitSettingsRoutesStockForgesAndPluginNamespaces(t *testing.T) {
	cfg := Defaults()
	cfg.GitHub.APIURL = "https://ghe.example.com/api/v3"
	cfg.GitHub.ServiceToken = "ghp_x"
	cfg.GitHub.App.ID = "123"
	cfg.GitLab.APIURL = "https://gitlab.example.com/api/v4"
	cfg.GitLab.ServiceToken = "glpat_x"
	cfg.GitLab.Viewer = "acme-bot"
	cfg.Gitea.URL = "https://git.example.com"
	cfg.Gitea.APIURL = "https://git.example.com/api/v1"
	cfg.Gitea.ServiceToken = "gta_x"
	cfg.Plugins = map[string]map[string]any{"bitbucket": {"api_url": "https://bitbucket.example.com/api/2.0"}}

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
	for _, tc := range []struct{ key, want string }{
		{"api_url", "https://gitlab.example.com/api/v4"},
		{"service_token", "glpat_x"},
		{"viewer", "acme-bot"},
		{"app.id", ""},
		{"nonesuch", ""},
	} {
		if got := gl(tc.key); got != tc.want {
			t.Errorf("gitlab setting %q = %q, want %q; a stock forge reads its own typed section, "+
				"not plugins.<ns>", tc.key, got, tc.want)
		}
	}

	gt := cfg.GitSettings("gitea")
	for _, tc := range []struct{ key, want string }{
		{"url", "https://git.example.com"},
		{"api_url", "https://git.example.com/api/v1"},
		{"service_token", "gta_x"},
		{"app.id", ""},
		{"nonesuch", ""},
	} {
		if got := gt(tc.key); got != tc.want {
			t.Errorf("gitea setting %q = %q, want %q; a stock forge reads its own typed section, "+
				"not plugins.<ns>", tc.key, got, tc.want)
		}
	}

	contributed := cfg.GitSettings("bitbucket")
	if got := contributed("api_url"); got != "https://bitbucket.example.com/api/2.0" {
		t.Errorf("bitbucket api_url = %q; a provider contributed by a plugin must read its settings from "+
			"its own plugins.<ns> namespace with no typed config field of its own", got)
	}
	if got := contributed("service_token"); got != "" {
		t.Errorf("unset contributed key = %q, want empty", got)
	}
}

func TestGitLabSettingsDoNotLeakFromGitHub(t *testing.T) {
	cfg := Defaults()
	cfg.GitHub.APIURL = "https://ghe.example.com/api/v3"
	cfg.GitHub.ServiceToken = "ghp_x"

	gl := cfg.GitSettings("gitlab")
	for _, key := range []string{"api_url", "service_token"} {
		if got := gl(key); got != "" {
			t.Errorf("gitlab %q = %q with only github configured; the two forges must not share "+
				"credentials or endpoints", key, got)
		}
	}
}
