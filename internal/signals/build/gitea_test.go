package build

import (
	"context"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/plugin"
	"github.com/codyconfer/mino/internal/signals/active"
	gt "github.com/codyconfer/mino/internal/signals/gitea"
)

func giteaCfg(t *testing.T, url string) *config.Config {
	t.Helper()
	t.Setenv("GITEA_TOKEN", "tok")
	t.Setenv("FORGEJO_TOKEN", "")
	cfg := config.Defaults()
	cfg.Home = t.TempDir()
	cfg.Git.Provider = "gitea"
	cfg.Gitea.URL = url
	return cfg
}

func TestGiteaSignalIsRegistered(t *testing.T) {
	if !HasBuilder("gitea") {
		t.Fatal("no gitea query builder; RegisterBuiltins declares the signal, so a missing builder is a dead descriptor")
	}
	if !HasActiveBuilder("gitea") {
		t.Fatal("no gitea stream builder, but the descriptor claims CapStream")
	}
	if !plugin.HasCapability("gitea", plugin.CapDetail) {
		t.Error("gitea does not declare CapDetail, so `mino gitea show` would not exist")
	}
	if keys := ParamKeys("gitea"); len(keys) == 0 {
		t.Error("no query params registered for gitea, so completion offers nothing")
	}
}

func TestBuildGiteaNeedsAnInstanceURL(t *testing.T) {
	cfg := giteaCfg(t, "")

	_, err := buildGitea(nil, cfg, nil, nil)
	if err == nil {
		t.Fatal("a gitea signal was built with no instance URL")
	}
	if errs.KindOf(err) != errs.KindConfig {
		t.Errorf("kind = %v, want %v", errs.KindOf(err), errs.KindConfig)
	}
	if !strings.Contains(err.Error()+errs.Hint(err), "gitea.url") {
		t.Errorf("error %q / hint %q should name gitea.url", err, errs.Hint(err))
	}
}

func TestGiteaBackendGetsTheAPIBase(t *testing.T) {
	cfg := giteaCfg(t, "https://git.example.com")

	backend, err := giteaBackend(cfg, nil)
	if err != nil {
		t.Fatalf("giteaBackend: %v", err)
	}
	api, ok := backend.(gt.APIBackend)
	if !ok {
		t.Fatalf("backend = %T, want the REST backend: gitea has no CLI to shell out to", backend)
	}
	if api.BaseURL != "https://git.example.com/api/v1" {
		t.Errorf("BaseURL = %q, want the instance root plus /api/v1", api.BaseURL)
	}
	tok, err := api.Auth.Token(context.Background())
	if err != nil || tok != "tok" {
		t.Errorf("Token() = %q/%v, want the ambient token", tok, err)
	}
}

func TestBuildGiteaRejectsABadQueryExpression(t *testing.T) {
	cfg := giteaCfg(t, "https://git.example.com")

	_, err := buildGitea(map[string]string{"query": "stat:open"}, cfg, nil, nil)
	if err == nil {
		t.Fatal("a bad query reached the API instead of failing at build time")
	}
	if errs.KindOf(err) != errs.KindConfig {
		t.Errorf("kind = %v, want %v", errs.KindOf(err), errs.KindConfig)
	}
}

func TestBuildActiveGiteaNeedsACredential(t *testing.T) {
	t.Setenv("GITEA_TOKEN", "")
	t.Setenv("FORGEJO_TOKEN", "")
	cfg := config.Defaults()
	cfg.Home = t.TempDir()
	cfg.Gitea.URL = "https://git.example.com"

	_, err := buildActiveGitea(nil, cfg, nil, active.NewState(nil))
	if errs.KindOf(err) != errs.KindAuth {
		t.Fatalf("err = %v, want an auth error", err)
	}
	if !strings.Contains(errs.Hint(err), "read:notification") {
		t.Errorf("hint = %q, want the scope the notification stream needs", errs.Hint(err))
	}
}

func TestBuildActiveGiteaEnforcesThePollFloor(t *testing.T) {
	cfg := giteaCfg(t, "https://git.example.com")

	if _, err := buildActiveGitea(map[string]string{"interval": "100ms"}, cfg, nil, active.NewState(nil)); err == nil {
		t.Error("an interval below the floor was accepted; a query param reaches the builder without passing the CLI flag")
	}
	if _, err := buildActiveGitea(map[string]string{"interval": "90s"}, cfg, nil, active.NewState(nil)); err != nil {
		t.Errorf("a valid interval was rejected: %v", err)
	}
}

func TestGiteaForgeNamesTheConfiguredProvider(t *testing.T) {
	cfg := giteaCfg(t, "https://git.example.com")
	if got := giteaForge(cfg); got != "gitea" {
		t.Errorf("giteaForge = %q, want gitea", got)
	}
	cfg.Git.Provider = "forgejo"
	if got := giteaForge(cfg); got != "forgejo" {
		t.Errorf("giteaForge = %q, want forgejo", got)
	}
}
