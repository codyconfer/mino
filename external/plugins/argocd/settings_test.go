package argocd

import (
	"strings"
	"testing"
)

func TestNormalizeServerURL(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    string
		wantErr string
	}{
		{name: "plain host", raw: "https://argocd.example.com", want: "https://argocd.example.com"},
		{name: "trailing slash", raw: "https://argocd.example.com/", want: "https://argocd.example.com"},
		{name: "api root pasted", raw: "https://argocd.example.com/api/v1", want: "https://argocd.example.com"},
		{name: "api prefix pasted", raw: "https://argocd.example.com/api", want: "https://argocd.example.com"},
		{name: "subpath preserved", raw: "https://ops.example.com/argocd", want: "https://ops.example.com/argocd"},
		{name: "query stripped", raw: "https://argocd.example.com/?x=1", want: "https://argocd.example.com"},
		{name: "surrounding space", raw: "  https://argocd.example.com  ", want: "https://argocd.example.com"},
		{name: "http refused", raw: "http://argocd.example.com", wantErr: "https"},
		{name: "empty", raw: "", wantErr: "server_url is required"},
		{name: "no host", raw: "https:///applications", wantErr: "no host"},
		{name: "not a url", raw: "https://%zz", wantErr: "not a URL"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := normalizeServerURL(c.raw)
			if c.wantErr != "" {
				if err == nil {
					t.Fatalf("normalizeServerURL(%q) = %q, want an error naming %q", c.raw, got, c.wantErr)
				}
				if !strings.Contains(err.Error(), c.wantErr) {
					t.Errorf("error = %q, want it to mention %q so the user can fix the config", err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeServerURL(%q): %v", c.raw, err)
			}
			if got != c.want {
				t.Errorf("normalizeServerURL(%q) = %q, want %q", c.raw, got, c.want)
			}
		})
	}
}

func TestHTTPRefusalNamesTheTokenRisk(t *testing.T) {
	_, err := normalizeServerURL("http://argocd.example.com")
	if err == nil {
		t.Fatal("plain http accepted; the bearer token would cross the wire in the clear")
	}
	if !strings.Contains(err.Error(), "token") {
		t.Errorf("error = %q, want it to explain that the refusal protects the API token", err)
	}
}

func TestConfigParamsBeatSettings(t *testing.T) {
	bc := buildCtx{
		params: map[string]string{
			"project":        "override",
			"selector":       "env=staging",
			"namespace":      "one,two",
			"only_unhealthy": "true",
			"group_by":       "cluster",
			"max":            "7",
			"app":            "payments-api",
		},
		settings: map[string]any{
			"server_url":     testServer,
			"projects":       []any{"from-settings"},
			"selector":       "env=prod",
			"namespaces":     []any{"ignored"},
			"only_unhealthy": false,
			"group_by":       "project",
			"max":            99,
		},
	}
	cfg, err := configFrom(bc)
	if err != nil {
		t.Fatalf("configFrom: %v", err)
	}
	if len(cfg.Projects) != 1 || cfg.Projects[0] != "override" {
		t.Errorf("Projects = %v, want the query param to win", cfg.Projects)
	}
	if cfg.Selector != "env=staging" {
		t.Errorf("Selector = %q, want the query param to win", cfg.Selector)
	}
	if len(cfg.Namespaces) != 2 {
		t.Errorf("Namespaces = %v, want the comma-separated param split into two", cfg.Namespaces)
	}
	if !cfg.OnlyUnhealthy {
		t.Error("OnlyUnhealthy = false, want the query param to win over the setting")
	}
	if cfg.GroupBy != groupByCluster {
		t.Errorf("GroupBy = %q, want %q", cfg.GroupBy, groupByCluster)
	}
	if cfg.Max != 7 {
		t.Errorf("Max = %d, want 7", cfg.Max)
	}
	if cfg.App != "payments-api" {
		t.Errorf("App = %q, want the app param carried through", cfg.App)
	}
}

func TestConfigFallsBackToSettingsThenDefaults(t *testing.T) {
	cfg, err := configFrom(buildCtx{settings: map[string]any{
		"server_url": testServer,
		"projects":   []any{"platform", "storefront"},
		"token_env":  "MY_ARGOCD_TOKEN",
	}})
	if err != nil {
		t.Fatalf("configFrom: %v", err)
	}
	if len(cfg.Projects) != 2 {
		t.Errorf("Projects = %v, want both settings entries", cfg.Projects)
	}
	if cfg.TokenEnv != "MY_ARGOCD_TOKEN" {
		t.Errorf("TokenEnv = %q, want the configured override", cfg.TokenEnv)
	}
	if cfg.Max != defaultMax {
		t.Errorf("Max = %d, want the default %d", cfg.Max, defaultMax)
	}
	if cfg.GroupBy != groupByNone {
		t.Errorf("GroupBy = %q, want %q", cfg.GroupBy, groupByNone)
	}
}

func TestUnknownGroupByFallsBackToNone(t *testing.T) {
	cfg, err := configFrom(buildCtx{settings: map[string]any{"server_url": testServer, "group_by": "team"}})
	if err != nil {
		t.Fatalf("configFrom: %v", err)
	}
	if cfg.GroupBy != groupByNone {
		t.Errorf("GroupBy = %q, want %q; an unrecognized grouping must not silently drop items",
			cfg.GroupBy, groupByNone)
	}
}

func TestNonPositiveMaxFallsBackToTheDefault(t *testing.T) {
	cfg, err := configFrom(buildCtx{settings: map[string]any{"server_url": testServer, "max": 0}})
	if err != nil {
		t.Fatalf("configFrom: %v", err)
	}
	if cfg.Max != defaultMax {
		t.Errorf("Max = %d, want %d; max=0 must not render an empty panel", cfg.Max, defaultMax)
	}
}

func TestAppURLForms(t *testing.T) {
	cases := []struct {
		name, appNamespace, want string
	}{
		{"payments-api", "", testServer + "/applications/payments-api"},
		{"payments-api", "argocd", testServer + "/applications/payments-api"},
		{"search-indexer", "team-search", testServer + "/applications/team-search/search-indexer"},
	}
	for _, c := range cases {
		if got := appURL(testServer, c.name, c.appNamespace); got != c.want {
			t.Errorf("appURL(%q, %q) = %q, want %q", c.name, c.appNamespace, got, c.want)
		}
	}
}

func TestServerHostStripsTheScheme(t *testing.T) {
	if got := serverHost(testServer); got != "argocd.example.com" {
		t.Errorf("serverHost = %q, want the bare host for the status chip", got)
	}
}
