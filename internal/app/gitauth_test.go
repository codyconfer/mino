package app

import (
	"context"
	"testing"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/gitauth"
)

type probeProvider struct{ env gitauth.Env }

func (p *probeProvider) Name() string  { return "probe" }
func (p *probeProvider) Label() string { return "Probe" }
func (p *probeProvider) Host() string  { return "probe.example.com" }

func (p *probeProvider) Resolve() (gitauth.Identity, error) { return probeIdentity{}, nil }

func (p *probeProvider) Status(context.Context, gitauth.Identity) gitauth.AuthStatus {
	return gitauth.AuthStatus{OK: true}
}

func (p *probeProvider) Account(context.Context, gitauth.Identity) (gitauth.Account, error) {
	return gitauth.Account{Login: "probe"}, nil
}

func (p *probeProvider) RateLimit(context.Context, gitauth.Identity) (gitauth.RateLimit, error) {
	return gitauth.RateLimit{}, nil
}

func (p *probeProvider) SigningKeyRegistered(context.Context, gitauth.Identity, gitauth.SigningKeyKind, string) gitauth.KeyCheck {
	return gitauth.KeyCheck{}
}

func (p *probeProvider) EmailVerified(context.Context, gitauth.Identity, string) (bool, error) {
	return false, nil
}

func (p *probeProvider) UploadKeyFix(gitauth.SigningKeyKind, string) []string { return nil }

func (p *probeProvider) Findings(context.Context, gitauth.Identity) []gitauth.Finding { return nil }

type probeIdentity struct{}

func (probeIdentity) Token(context.Context) (string, error) { return "probe-token", nil }
func (probeIdentity) Origin() string                        { return "the probe" }
func (probeIdentity) Authenticated() bool                   { return true }
func (probeIdentity) ServiceIdentity() bool                 { return false }
func (probeIdentity) Trace() string                         { return "probe: auth tiers: -> selected probe" }
func (probeIdentity) Invalidate()                           {}

func registerProbe(t *testing.T, name string) *gitauth.Env {
	t.Helper()
	var captured gitauth.Env
	gitauth.Register(name, func(env gitauth.Env) (gitauth.Provider, error) {
		captured = env
		return &probeProvider{env: env}, nil
	})
	return &captured
}

func TestResolveGitAuthDoesNotLeakGitHubsAPIURLIntoOtherProviders(t *testing.T) {
	env := registerProbe(t, "probe-leak")
	a := &App{Cfg: &config.Config{
		Git:    config.GitConfig{Provider: "probe-leak"},
		GitHub: config.GitHubConfig{APIURL: "https://ghe.example.com/api/v3"},
	}}

	if _, _, err := a.GitAuth(); err != nil {
		t.Fatalf("GitAuth: %v", err)
	}
	if got := env.Get("api_url"); got != "" {
		t.Errorf("provider read api_url=%q from the github section; a second forge would ship its own token to %q", got, got)
	}
}

func TestResolveGitAuthStillNormalizesGitHubsAPIURL(t *testing.T) {
	a := &App{Cfg: &config.Config{
		GitHub: config.GitHubConfig{APIURL: "https://ghe.example.com/api/v3/"},
	}}

	p, _, err := a.GitAuth()
	if err != nil {
		t.Fatalf("GitAuth: %v", err)
	}
	if p == nil {
		t.Fatal("no provider resolved")
	}
	if got := p.Host(); got != "ghe.example.com" {
		t.Errorf("Host() = %q, want ghe.example.com; the enterprise api_url must still reach the github provider", got)
	}
}

func TestResolveGitAuthRejectsAnInsecureGitHubAPIURL(t *testing.T) {
	a := &App{Cfg: &config.Config{
		GitHub: config.GitHubConfig{APIURL: "http://ghe.example.com/api/v3"},
	}}

	_, _, err := a.GitAuth()
	if err == nil {
		t.Fatal("an http api_url was accepted")
	}
	if errs.KindOf(err) != errs.KindConfig {
		t.Errorf("kind = %v, want %v", errs.KindOf(err), errs.KindConfig)
	}
}

func TestResolveGitAuthIgnoresAnInvalidGitHubAPIURLForAnotherProvider(t *testing.T) {
	registerProbe(t, "probe-unaffected")
	a := &App{Cfg: &config.Config{
		Git:    config.GitConfig{Provider: "probe-unaffected"},
		GitHub: config.GitHubConfig{APIURL: "http://insecure.example.com"},
	}}

	if _, _, err := a.GitAuth(); err != nil {
		t.Fatalf("a broken github.api_url must not break another provider: %v", err)
	}
}
