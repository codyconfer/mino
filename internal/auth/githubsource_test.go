package auth

import (
	"context"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/errs"
)

func clearAmbientGitHub(t *testing.T) {
	t.Helper()
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	t.Setenv(envAppKey, "")
}

func withoutGHOnPath(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
}

func TestSelectGitHubPrefersServiceAuthOverEverythingAmbient(t *testing.T) {
	clearAmbientGitHub(t)
	fakeGH(t, "#!/bin/sh\necho gh-token\n")
	t.Setenv("GITHUB_TOKEN", "env-token")
	store := memStore{"github": Credential{AccessToken: "stored-token"}}

	sel, err := SelectGitHub(GitHubSpec{ServiceToken: "service-token", Store: store})
	if err != nil {
		t.Fatalf("SelectGitHub: %v", err)
	}
	if sel.Mech != GitHubServiceToken {
		t.Fatalf("mech = %v, want service token; a configured service identity that loses to the gh CLI or "+
			"$GITHUB_TOKEN means mino acts as a human when it was told to act as a service", sel.Mech)
	}
	if sel.Origin != "github.service_token" {
		t.Errorf("origin = %q, want github.service_token", sel.Origin)
	}
	tok, err := sel.Token(context.Background())
	if err != nil || tok != "service-token" {
		t.Errorf("Token() = %q, %v; want service-token", tok, err)
	}
	if sel.UsesGHCLI() {
		t.Error("UsesGHCLI() is true under service auth; GHAPIGet would shell out to `gh api` and silently " +
			"use the wrong identity")
	}
}

func TestSelectGitHubFallsBackThroughTheAmbientTiers(t *testing.T) {
	for _, tc := range []struct {
		name       string
		env        map[string]string
		store      memStore
		wantMech   GitHubMechanism
		wantOrigin string
		wantToken  string
	}{
		{
			name:       "GITHUB_TOKEN",
			env:        map[string]string{"GITHUB_TOKEN": "a", "GH_TOKEN": "b"},
			store:      memStore{"github": Credential{AccessToken: "c"}},
			wantMech:   GitHubEnvToken,
			wantOrigin: originGitHubToken,
			wantToken:  "a",
		},
		{
			name:       "GH_TOKEN",
			env:        map[string]string{"GH_TOKEN": "b"},
			store:      memStore{"github": Credential{AccessToken: "c"}},
			wantMech:   GitHubEnvToken,
			wantOrigin: originGHToken,
			wantToken:  "b",
		},
		{
			name:       "sealed store",
			store:      memStore{"github": Credential{AccessToken: "c"}},
			wantMech:   GitHubStoredToken,
			wantOrigin: originStoredToken,
			wantToken:  "c",
		},
		{
			name:     "nothing",
			wantMech: GitHubNone,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearAmbientGitHub(t)
			withoutGHOnPath(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			sel, err := SelectGitHub(GitHubSpec{Store: tc.store})
			if err != nil {
				t.Fatalf("SelectGitHub: %v", err)
			}
			if sel.Mech != tc.wantMech {
				t.Errorf("mech = %v, want %v", sel.Mech, tc.wantMech)
			}
			if sel.Origin != tc.wantOrigin {
				t.Errorf("origin = %q, want %q; the origin is user-visible in status text and logs",
					sel.Origin, tc.wantOrigin)
			}
			if tc.wantToken == "" {
				return
			}
			if tok, err := sel.Token(context.Background()); err != nil || tok != tc.wantToken {
				t.Errorf("Token() = %q, %v; want %q", tok, err, tc.wantToken)
			}
		})
	}
}

func TestSelectGitHubUsesTheStoredServiceCredentialAheadOfTheGHCLI(t *testing.T) {
	clearAmbientGitHub(t)
	fakeGH(t, "#!/bin/sh\necho gh-token\n")
	store := memStore{serviceCredKey: Credential{AccessToken: "svc"}}

	sel, err := SelectGitHub(GitHubSpec{Store: store})
	if err != nil {
		t.Fatalf("SelectGitHub: %v", err)
	}
	if sel.Mech != GitHubServiceToken {
		t.Fatalf("mech = %v, want service token from the sealed store", sel.Mech)
	}
	if !strings.Contains(sel.Origin, serviceCredKey) {
		t.Errorf("origin = %q; want it to name the %q store key so the identity is traceable",
			sel.Origin, serviceCredKey)
	}
}

func TestSelectGitHubUnauthenticatedTokenExplainsEveryOption(t *testing.T) {
	clearAmbientGitHub(t)
	withoutGHOnPath(t)

	sel, err := SelectGitHub(GitHubSpec{})
	if err != nil {
		t.Fatalf("SelectGitHub: %v", err)
	}
	_, err = sel.Token(context.Background())
	if err == nil {
		t.Fatal("Token() succeeded with nothing configured")
	}
	hint := errs.Hint(err)
	for _, want := range []string{"github.app", "github.service_token", "gh auth login", "GITHUB_TOKEN"} {
		if !strings.Contains(hint, want) {
			t.Errorf("hint does not mention %q: %q", want, hint)
		}
	}
}

func TestMisconfiguredAppFailsInsteadOfFallingBack(t *testing.T) {
	for _, tc := range []struct {
		name    string
		app     GitHubAppSpec
		wantMsg string
	}{
		{"no id", GitHubAppSpec{PrivateKeyPath: "/nope.pem"}, "github.app.id"},
		{"non-numeric id", GitHubAppSpec{ID: "my-app", PrivateKeyPath: "/nope.pem"}, "not numeric"},
		{"no key", GitHubAppSpec{ID: "123"}, "no private key"},
		{"bad installation id", GitHubAppSpec{ID: "123", InstallationID: "acme", PrivateKeyPath: "/nope.pem"}, "installation_id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearAmbientGitHub(t)
			fakeGH(t, "#!/bin/sh\necho gh-token\n")
			t.Setenv("GITHUB_TOKEN", "a-human-token")

			sel, err := SelectGitHub(GitHubSpec{App: tc.app, Store: memStore{}})
			if err == nil {
				t.Fatalf("SelectGitHub succeeded with a half-configured app and selected %v; falling back "+
					"to a human credential would run flights, actions and comments as that person and move "+
					"rate-limit accounting to them", sel.Mech)
			}
			if errs.KindOf(err) != errs.KindConfig {
				t.Errorf("kind = %v, want KindConfig", errs.KindOf(err))
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("error = %q; want it to name %q so the user knows which field to fix", err, tc.wantMsg)
			}
			if sel.Authenticated() {
				t.Error("the returned selection is authenticated despite the error")
			}
		})
	}
}

func TestSelectGitHubTraceNamesEveryTierAndTheWinner(t *testing.T) {
	clearAmbientGitHub(t)
	fakeGH(t, "#!/bin/sh\necho gh-token\n")
	t.Setenv("GITHUB_TOKEN", "env-token")

	sel, err := SelectGitHub(GitHubSpec{Store: memStore{}})
	if err != nil {
		t.Fatalf("SelectGitHub: %v", err)
	}
	trace := sel.Trace()
	for _, want := range []string{"app=unset", "service_token=unset", "gh=available", "-> selected gh CLI"} {
		if !strings.Contains(trace, want) {
			t.Errorf("trace is missing %q, so a user cannot tell why their $GITHUB_TOKEN was ignored: %q",
				want, trace)
		}
	}
}

func TestGitHubOriginNeverContainsTheToken(t *testing.T) {
	clearAmbientGitHub(t)
	withoutGHOnPath(t)
	const secret = "ghp_do_not_leak_me"

	for _, spec := range []GitHubSpec{
		{ServiceToken: secret, Store: memStore{}},
		{Store: memStore{serviceCredKey: Credential{AccessToken: secret}}},
		{Store: memStore{"github": Credential{AccessToken: secret}}},
	} {
		sel, err := SelectGitHub(spec)
		if err != nil {
			t.Fatalf("SelectGitHub: %v", err)
		}
		if strings.Contains(sel.Origin, secret) || strings.Contains(sel.Trace(), secret) {
			t.Errorf("the token leaked into origin %q or trace %q; both are logged and shown in status text",
				sel.Origin, sel.Trace())
		}
	}
}

func TestServiceIdentityClassification(t *testing.T) {
	for _, tc := range []struct {
		mech        GitHubMechanism
		wantService bool
	}{
		{GitHubAppAuth, true},
		{GitHubServiceToken, true},
		{GitHubCLI, false},
		{GitHubEnvToken, false},
		{GitHubStoredToken, false},
		{GitHubNone, false},
	} {
		sel := GitHubSelection{Mech: tc.mech}
		if sel.ServiceIdentity() != tc.wantService {
			t.Errorf("%v: ServiceIdentity() = %v, want %v; this decides whether @me is warned about and "+
				"whether the onboarding signing checks apply, so a misclassification is either a false "+
				"alarm for a human or a silent empty result set for a service",
				tc.mech, sel.ServiceIdentity(), tc.wantService)
		}
	}
}

func ambientSelection(t *testing.T, apiURL string) GitHubSelection {
	t.Helper()
	sel, err := SelectGitHub(GitHubSpec{APIURL: apiURL, Store: memStore{}})
	if err != nil {
		t.Fatalf("SelectGitHub: %v", err)
	}
	return sel
}
