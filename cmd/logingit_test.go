package cmd

import (
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/app/loginflow"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/testenv"
)

func sharedWithServiceAuth(t *testing.T, cfg *config.Config) {
	t.Helper()
	testenv.Isolate(t)
	orig := shared
	t.Cleanup(func() { shared = orig })
	cfg.Home = t.TempDir()
	shared = &app.App{Cfg: cfg, Directives: &config.Directives{}}
	closeSharedDBs(t)
}

func TestServiceAuthRefusalOnlyGuardsTheConfiguredForge(t *testing.T) {
	t.Setenv("GITEA_TOKEN", "")
	t.Setenv("FORGEJO_TOKEN", "")
	sharedWithServiceAuth(t, &config.Config{
		Output: "terminal",
		GitHub: config.GitHubConfig{ServiceToken: "ghp_service"},
		Gitea:  config.GiteaConfig{URL: "https://git.example.com"},
	})

	gh, err := loginflow.ResolveOrErr("github")
	if err != nil {
		t.Fatal(err)
	}
	err = refuseWhenServiceAuthWins(gh)
	if err == nil {
		t.Fatal("a personal github login was allowed while a github service token is configured")
	}
	if !strings.Contains(errs.Hint(err), "github.service_token") {
		t.Errorf("hint = %q, want it to name the github field to unset", errs.Hint(err))
	}

	gitea, err := loginflow.ResolveOrErr("gitea")
	if err != nil {
		t.Fatal(err)
	}
	if err := refuseWhenServiceAuthWins(gitea); err != nil {
		t.Fatalf("`mino login gitea` was refused because github has a service token: %v", err)
	}
}

func TestServiceAuthRefusalNamesGiteaFieldsWhenGiteaIsConfigured(t *testing.T) {
	t.Setenv("GITEA_TOKEN", "")
	t.Setenv("FORGEJO_TOKEN", "")
	sharedWithServiceAuth(t, &config.Config{
		Output: "terminal",
		Git:    config.GitConfig{Provider: "forgejo"},
		Gitea:  config.GiteaConfig{URL: "https://git.example.com", ServiceToken: "gta_service"},
	})

	p, err := loginflow.ResolveOrErr("forgejo")
	if err != nil {
		t.Fatal(err)
	}
	err = refuseWhenServiceAuthWins(p)
	if err == nil {
		t.Fatal("a personal login was allowed while gitea service auth is configured")
	}
	if hint := errs.Hint(err); !strings.Contains(hint, "gitea.service_token") || !strings.Contains(hint, "MINO_GITEA_SERVICE_TOKEN") {
		t.Errorf("hint = %q, want the gitea field names rather than github's", hint)
	}
}

func TestServiceAuthRefusalIgnoresNonForgeProviders(t *testing.T) {
	sharedWithServiceAuth(t, &config.Config{
		Output: "terminal",
		GitHub: config.GitHubConfig{ServiceToken: "ghp_service"},
	})

	if err := refuseWhenServiceAuthWins(loginflow.Provider{Key: "slack", Label: "Slack"}); err != nil {
		t.Fatalf("a slack login was refused by a git service credential: %v", err)
	}
}
