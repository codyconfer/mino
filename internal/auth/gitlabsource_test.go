package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/errs"
)

func TestSelectGitLabPrefersServiceAuthOverEverythingAmbient(t *testing.T) {
	fakeGLab(t, "#!/bin/sh\necho cli-token\n")
	t.Setenv("GITLAB_TOKEN", "env-token")
	store := memStore{gitlabCredKey: {AccessToken: "stored-token"}}

	sel, err := SelectGitLab(GitLabSpec{ServiceToken: "glpat-service", Store: store})
	if err != nil {
		t.Fatal(err)
	}
	if sel.Mech != GitLabServiceToken || sel.Origin != "gitlab.service_token" {
		t.Fatalf("mech = %v origin = %q, want the configured service token", sel.Mech, sel.Origin)
	}
	tok, err := sel.Token(context.Background())
	if err != nil || tok != "glpat-service" {
		t.Errorf("token = %q/%v; configuring a service identity is a commitment, not a preference, "+
			"so nothing ambient may outrank it", tok, err)
	}
	if !sel.ServiceIdentity() {
		t.Error("a configured service token must classify as a service identity")
	}
}

func TestSelectGitLabUsesTheStoredServiceCredentialAheadOfTheGLabCLI(t *testing.T) {
	fakeGLab(t, "#!/bin/sh\necho cli-token\n")
	store := memStore{gitlabServiceCredKey: {AccessToken: "sealed-service"}}

	sel, err := SelectGitLab(GitLabSpec{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	if sel.Mech != GitLabServiceToken {
		t.Fatalf("mech = %v, want the sealed service credential", sel.Mech)
	}
	if !strings.Contains(sel.Origin, gitlabServiceCredKey) {
		t.Errorf("origin = %q, want it to name the sealed store key", sel.Origin)
	}
}

func TestSelectGitLabFallsBackThroughTheAmbientTiers(t *testing.T) {
	withoutGLabOnPath(t)
	store := memStore{}

	clearAmbientGitLab(t)
	sel, err := SelectGitLab(GitLabSpec{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	if sel.Authenticated() {
		t.Fatalf("mech = %v with nothing configured, want none", sel.Mech)
	}

	store[gitlabCredKey] = Credential{AccessToken: "stored-token"}
	sel, _ = SelectGitLab(GitLabSpec{Store: store})
	if sel.Mech != GitLabStoredToken || sel.Origin != originGitLabStored {
		t.Errorf("mech = %v origin = %q, want the stored OAuth token", sel.Mech, sel.Origin)
	}

	t.Setenv("GL_TOKEN", "gl-token")
	sel, _ = SelectGitLab(GitLabSpec{Store: store})
	if sel.Mech != GitLabEnvToken || sel.Origin != originGLToken {
		t.Errorf("mech = %v origin = %q, want $GL_TOKEN to outrank the store", sel.Mech, sel.Origin)
	}

	t.Setenv("GITLAB_TOKEN", "gitlab-token")
	sel, _ = SelectGitLab(GitLabSpec{Store: store})
	if sel.Origin != originGitLabToken {
		t.Errorf("origin = %q, want $GITLAB_TOKEN to outrank $GL_TOKEN", sel.Origin)
	}
	tok, _ := sel.Token(context.Background())
	if tok != "gitlab-token" {
		t.Errorf("token = %q, want gitlab-token", tok)
	}
}

func TestSelectGitLabPrefersTheCLIOverAmbientEnv(t *testing.T) {
	fakeGLab(t, "#!/bin/sh\necho cli-token\n")
	t.Setenv("GITLAB_TOKEN", "env-token")

	sel, err := SelectGitLab(GitLabSpec{Store: memStore{}})
	if err != nil {
		t.Fatal(err)
	}
	if sel.Mech != GitLabCLI {
		t.Fatalf("mech = %v, want the glab CLI; glab honours $GITLAB_TOKEN itself, so preferring it "+
			"matches what the user's own CLI would do", sel.Mech)
	}
	tok, err := sel.Token(context.Background())
	if err != nil || tok != "cli-token" {
		t.Errorf("token = %q/%v, want the value glab printed", tok, err)
	}
}

func TestGLabCLISourceExplainsAnEmptyToken(t *testing.T) {
	fakeGLab(t, "#!/bin/sh\nexit 0\n")

	sel, _ := SelectGitLab(GitLabSpec{Store: memStore{}})
	_, err := sel.Token(context.Background())
	if err == nil {
		t.Fatal("an empty glab token was accepted")
	}
	if !strings.Contains(err.Error()+" ", "glab") {
		t.Errorf("error = %v, want it to name glab", err)
	}
}

func TestSelectGitLabTraceNamesEveryTierAndTheWinner(t *testing.T) {
	fakeGLab(t, "#!/bin/sh\necho cli-token\n")

	sel, _ := SelectGitLab(GitLabSpec{Store: memStore{}})
	for _, want := range []string{
		"service_token=unset",
		"store:" + gitlabServiceCredKey + "=miss",
		"glab=available",
		"-> selected glab CLI",
	} {
		if !strings.Contains(sel.Trace(), want) {
			t.Errorf("trace %q is missing %q; `mino verify auth -v` must show which tiers lost and why",
				sel.Trace(), want)
		}
	}
}

func TestSelectGitLabTraceEndsInNoneWhenNothingResolves(t *testing.T) {
	withoutGLabOnPath(t)
	clearAmbientGitLab(t)

	sel, _ := SelectGitLab(GitLabSpec{Store: memStore{}})
	if !strings.Contains(sel.Trace(), "-> none") {
		t.Errorf("trace = %q, want it to end in -> none", sel.Trace())
	}
	if !strings.Contains(sel.Trace(), "ambient=none") {
		t.Errorf("trace = %q, want the ambient tier recorded", sel.Trace())
	}
}

func TestGitLabOriginNeverContainsTheToken(t *testing.T) {
	withoutGLabOnPath(t)
	t.Setenv("GITLAB_TOKEN", "glpat-supersecret")
	t.Setenv("GL_TOKEN", "")

	sel, _ := SelectGitLab(GitLabSpec{ServiceToken: "", Store: memStore{}})
	if strings.Contains(sel.Origin, "supersecret") || strings.Contains(sel.Trace(), "supersecret") {
		t.Fatalf("the token leaked into origin %q or trace %q; both are logged", sel.Origin, sel.Trace())
	}
}

func TestSelectGitLabUnauthenticatedTokenExplainsEveryOption(t *testing.T) {
	withoutGLabOnPath(t)
	clearAmbientGitLab(t)

	sel, _ := SelectGitLab(GitLabSpec{Store: memStore{}})
	_, err := sel.Token(context.Background())
	if err == nil {
		t.Fatal("an unauthenticated selection returned a token")
	}
	for _, want := range []string{"gitlab.service_token", "glab auth login", "GITLAB_TOKEN", "mino login gitlab"} {
		if !strings.Contains(err.Error()+" "+errs.Hint(err), want) {
			t.Errorf("the error does not mention %q, leaving the user to guess: %v", want, err)
		}
	}
}

func TestGitLabServiceIdentityClassification(t *testing.T) {
	cases := []struct {
		mech GitLabMechanism
		want bool
	}{
		{GitLabServiceToken, true},
		{GitLabCLI, false},
		{GitLabEnvToken, false},
		{GitLabStoredToken, false},
		{GitLabNone, false},
	}
	for _, c := range cases {
		sel := GitLabSelection{Mech: c.mech}
		if got := sel.ServiceIdentity(); got != c.want {
			t.Errorf("%v ServiceIdentity = %v, want %v; only a service tier may suppress the "+
				"personal-credential fallbacks", c.mech, got, c.want)
		}
	}
}

func TestStoredTokenRefreshesBeforeExpiryAndRewritesTheStore(t *testing.T) {
	withoutGLabOnPath(t)
	clearAmbientGitLab(t)

	var refreshes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refreshes++
		if got := r.FormValue("grant_type"); got != "refresh_token" {
			t.Errorf("grant_type = %q, want refresh_token", got)
		}
		if got := r.FormValue("refresh_token"); got != "rt-1" {
			t.Errorf("refresh_token = %q, want rt-1", got)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "fresh", "refresh_token": "rt-2", "expires_in": 7200, "scope": "read_api",
		})
	}))
	defer srv.Close()

	store := memStore{gitlabCredKey: {
		AccessToken:  "stale",
		RefreshToken: "rt-1",
		Expiry:       time.Now().Add(10 * time.Second),
	}}
	sel, _ := SelectGitLab(GitLabSpec{APIURL: srv.URL + "/api/v4", OAuthClientID: "cid", Store: store})
	if sel.Mech != GitLabStoredToken {
		t.Fatalf("mech = %v, want the stored token", sel.Mech)
	}

	tok, err := sel.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok != "fresh" {
		t.Fatalf("token = %q, want the refreshed one; GitLab access tokens last two hours", tok)
	}
	if got := store[gitlabCredKey].AccessToken; got != "fresh" {
		t.Errorf("stored token = %q, want it rewritten; otherwise every process refreshes again", got)
	}
	if got := store[gitlabCredKey].RefreshToken; got != "rt-2" {
		t.Errorf("stored refresh token = %q, want the rotated one", got)
	}

	if _, err := sel.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if refreshes != 1 {
		t.Errorf("refreshed %d times for two Token calls, want 1", refreshes)
	}
}

func TestStoredTokenWithoutARefreshTokenIsUsedAsIs(t *testing.T) {
	withoutGLabOnPath(t)
	clearAmbientGitLab(t)

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("a token with no refresh token must not trigger a network call")
	}))
	defer srv.Close()

	store := memStore{gitlabCredKey: {AccessToken: "only", Expiry: time.Now().Add(time.Second)}}
	sel, _ := SelectGitLab(GitLabSpec{APIURL: srv.URL + "/api/v4", OAuthClientID: "cid", Store: store})
	tok, err := sel.Token(context.Background())
	if err != nil || tok != "only" {
		t.Fatalf("token = %q/%v, want the stored value returned so the API's 401 speaks", tok, err)
	}
}

func TestStoredTokenWellBeforeExpiryDoesNotRefresh(t *testing.T) {
	withoutGLabOnPath(t)
	clearAmbientGitLab(t)

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("refreshed a token that is nowhere near expiry")
	}))
	defer srv.Close()

	store := memStore{gitlabCredKey: {
		AccessToken:  "current",
		RefreshToken: "rt-1",
		Expiry:       time.Now().Add(90 * time.Minute),
	}}
	sel, _ := SelectGitLab(GitLabSpec{APIURL: srv.URL + "/api/v4", OAuthClientID: "cid", Store: store})
	if tok, err := sel.Token(context.Background()); err != nil || tok != "current" {
		t.Fatalf("token = %q/%v, want the cached value", tok, err)
	}
}
