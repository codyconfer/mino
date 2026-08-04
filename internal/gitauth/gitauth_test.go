package gitauth

import (
	"context"
	"strings"
	"testing"
)

type stubProvider struct {
	name string
	env  Env
}

func (s *stubProvider) Name() string               { return s.name }
func (s *stubProvider) Label() string              { return strings.ToUpper(s.name) }
func (s *stubProvider) Host() string               { return s.name + ".test" }
func (s *stubProvider) Resolve() (Identity, error) { return stubIdentity{}, nil }
func (s *stubProvider) Status(context.Context, Identity) AuthStatus {
	return AuthStatus{OK: true, Detail: "stub"}
}
func (s *stubProvider) Account(context.Context, Identity) (Account, error) {
	return Account{Login: s.env.Get("login")}, nil
}
func (s *stubProvider) RateLimit(context.Context, Identity) (RateLimit, error) {
	return RateLimit{}, nil
}
func (s *stubProvider) SigningKeyRegistered(context.Context, Identity, SigningKeyKind, string) KeyCheck {
	return KeyCheck{Registered: true}
}
func (s *stubProvider) EmailVerified(context.Context, Identity, string) (bool, error) {
	return true, nil
}
func (s *stubProvider) UploadKeyFix(SigningKeyKind, string) []string { return nil }
func (s *stubProvider) Findings(context.Context, Identity) []Finding { return nil }

type stubIdentity struct{}

func (stubIdentity) Token(context.Context) (string, error) { return "t", nil }
func (stubIdentity) Origin() string                        { return "stub" }
func (stubIdentity) Authenticated() bool                   { return true }
func (stubIdentity) ServiceIdentity() bool                 { return false }
func (stubIdentity) Trace() string                         { return "stub trace" }
func (stubIdentity) Invalidate()                           {}

func TestARegisteredProviderCanReplaceTheDefault(t *testing.T) {
	Register("stubforge", func(env Env) (Provider, error) {
		return &stubProvider{name: "stubforge", env: env}, nil
	})

	p, err := New("stubforge", Env{Setting: func(k string) string {
		if k == "login" {
			return "stub-bot"
		}
		return ""
	}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if p.Name() != "stubforge" || p.Host() != "stubforge.test" {
		t.Errorf("provider = %s at %s, want the registered stub", p.Name(), p.Host())
	}
	id, err := p.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	acct, err := p.Account(context.Background(), id)
	if err != nil {
		t.Fatalf("Account: %v", err)
	}
	if acct.Login != "stub-bot" {
		t.Errorf("login = %q, want stub-bot; a provider must read its own settings through Env, so that "+
			"adding a forge needs no change in internal/app or internal/config", acct.Login)
	}
}

func TestUnknownProviderNamesWhatIsAvailable(t *testing.T) {
	_, err := New("nosuchforge", Env{})
	if err == nil {
		t.Fatal("New accepted an unregistered provider")
	}
	if !strings.Contains(err.Error(), "nosuchforge") {
		t.Errorf("error does not name the bad provider: %v", err)
	}
	if !strings.Contains(err.Error(), "known") {
		t.Errorf("error does not list the known providers, leaving the user to guess: %v", err)
	}
}

func TestEmptyNameResolvesToTheDefault(t *testing.T) {
	if Default != "github" {
		t.Fatalf("Default = %q, want github; an unset git.provider must keep every existing install "+
			"working exactly as before", Default)
	}
}

func TestEnvGetIsSafeWithNoSetting(t *testing.T) {
	var e Env
	if got := e.Get("anything"); got != "" {
		t.Errorf("Get on a zero Env = %q, want empty; a provider built without settings must not panic", got)
	}
}
