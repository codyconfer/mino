package loginflow

import (
	"testing"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/config"
)

func appWithProvider(t *testing.T, provider string) *app.App {
	t.Helper()
	cfg := config.Defaults()
	cfg.Home = t.TempDir()
	cfg.Git.Provider = provider
	return &app.App{Cfg: cfg, Directives: &config.Directives{}}
}

func provider(t *testing.T, key string) Provider {
	t.Helper()
	p, ok := Resolve(key)
	if !ok {
		t.Fatalf("Resolve(%q) not found", key)
	}
	return p
}

func TestGitLabIsACoreLoginProvider(t *testing.T) {
	p := provider(t, "gitlab")
	if p.Label != "GitLab" {
		t.Errorf("label = %q", p.Label)
	}
	if len(p.Fields) != 1 || p.Fields[0].Key != "gitlab.oauth_client_id" {
		t.Errorf("fields = %+v, want the OAuth application id", p.Fields)
	}
	if p.Login == nil || p.Authed == nil {
		t.Error("the gitlab provider is not runnable")
	}
}

func TestGitLabMissingReportsTheClientID(t *testing.T) {
	a := appWithProvider(t, "gitlab")
	missing := provider(t, "gitlab").Missing(a)
	if len(missing) != 1 || missing[0].Key != "gitlab.oauth_client_id" {
		t.Fatalf("missing = %+v, want the client id prompted for", missing)
	}

	a.Cfg.GitLab.OAuthClientID = "abc123"
	if got := provider(t, "gitlab").Missing(a); len(got) != 0 {
		t.Errorf("missing = %+v once the client id is set", got)
	}
}

func TestLoginProviderAuthedIgnoresTheOtherForge(t *testing.T) {
	a := appWithProvider(t, "gitlab")
	defer a.CloseDBs()

	// a.GitAuthed() reports on git.provider, which is gitlab here. Asking it about github
	// would answer the wrong question and make `mino login github` refuse to run.
	if provider(t, "github").Authed(a) {
		t.Error("the github provider reported authenticated on a git.provider: gitlab install with " +
			"no github credential stored")
	}
}

func TestActiveGitProviderFallsBackToTheDefault(t *testing.T) {
	if got := ActiveGitProvider(nil); got != "github" {
		t.Errorf("ActiveGitProvider(nil) = %q, want the default", got)
	}
	a := appWithProvider(t, "")
	defer a.CloseDBs()
	if got := ActiveGitProvider(a); got != "github" {
		t.Errorf("unset git.provider = %q, want github", got)
	}
	b := appWithProvider(t, "gitlab")
	defer b.CloseDBs()
	if got := ActiveGitProvider(b); got != "gitlab" {
		t.Errorf("git.provider: gitlab = %q", got)
	}
}

func TestNamesIncludeBothForges(t *testing.T) {
	names := Names()
	for _, want := range []string{"github", "gitlab"} {
		var found bool
		for _, n := range names {
			if n == want {
				found = true
			}
		}
		if !found {
			t.Errorf("Names() = %v, want %q so `mino login %s` completes", names, want, want)
		}
	}
}
