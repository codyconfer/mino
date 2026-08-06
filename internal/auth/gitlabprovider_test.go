package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/gitauth"
)

const gitlabUserFixture = `{
  "id": 42, "username": "codyconfer", "name": "Cody Confer",
  "email": "cody@example.com", "state": "active"
}`

const gitlabSSHKeysFixture = `[
  {"id": 1, "title": "laptop", "key": "ssh-ed25519 AAAAKEYBODY laptop@home", "usage_type": "signing"},
  {"id": 2, "title": "auth only", "key": "ssh-ed25519 AAAAAUTHONLY box", "usage_type": "auth"},
  {"id": 3, "title": "legacy", "key": "ssh-ed25519 AAAALEGACY old"}
]`

const gitlabEmailsFixture = `[
  {"id": 1, "email": "secondary@example.com", "confirmed_at": "2026-01-01T00:00:00Z"},
  {"id": 2, "email": "unconfirmed@example.com", "confirmed_at": null}
]`

func gitlabAPI(t *testing.T, routes map[string]http.HandlerFunc) (*gitlabProvider, gitauth.Identity) {
	t.Helper()
	withoutGLabOnPath(t)
	clearAmbientGitLab(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h, ok := routes[strings.TrimPrefix(r.URL.Path, "/api/v4/")]
		if !ok {
			http.Error(w, `{"message":"404 Not Found"}`, http.StatusNotFound)
			return
		}
		h(w, r)
	}))
	t.Cleanup(srv.Close)

	p := &gitlabProvider{spec: GitLabSpec{
		APIURL:       srv.URL + "/api/v4",
		ServiceToken: "glpat-test",
		Store:        memStore{},
	}}
	id, err := p.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	return p, id
}

func readTrimmed(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func nowPlusHours(n int) time.Time { return time.Now().Add(time.Duration(n) * time.Hour) }

func jsonRoute(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}
}

func TestGitLabIsARegisteredGitProvider(t *testing.T) {
	if !gitauth.Known("gitlab") {
		t.Fatal("gitauth does not know gitlab; git.provider: gitlab would fail at startup")
	}
	p, err := gitauth.New("gitlab", gitauth.Env{Setting: func(string) string { return "" }})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "gitlab" || p.Label() != "GitLab" || p.Host() != defaultGitLabHost {
		t.Errorf("provider = %s/%s at %s", p.Name(), p.Label(), p.Host())
	}
}

func TestGitLabProviderRejectsAPlainHTTPAPIURL(t *testing.T) {
	_, err := gitauth.New("gitlab", gitauth.Env{Setting: func(k string) string {
		if k == "api_url" {
			return "http://gitlab.example.com"
		}
		return ""
	}})
	if err == nil {
		t.Fatal("a plain-http api_url was accepted; the token would go out in the clear")
	}
	if errs.KindOf(err) != errs.KindConfig {
		t.Errorf("kind = %v, want config", errs.KindOf(err))
	}
}

func TestGitLabProviderHostFollowsTheAPIURL(t *testing.T) {
	p := &gitlabProvider{spec: GitLabSpec{APIURL: "https://gitlab.example.com/api/v4"}}
	if p.Host() != "gitlab.example.com" {
		t.Errorf("Host = %q, want the self-managed host", p.Host())
	}
}

func TestGitLabAccountReadsUsername(t *testing.T) {
	p, id := gitlabAPI(t, map[string]http.HandlerFunc{"user": jsonRoute(gitlabUserFixture)})

	acct, err := p.Account(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if acct.Login != "codyconfer" {
		t.Errorf("login = %q, want the username; GitLab has no login field", acct.Login)
	}
}

func TestGitLabAccountWorksForAServiceToken(t *testing.T) {
	p, id := gitlabAPI(t, map[string]http.HandlerFunc{
		"user": jsonRoute(`{"id": 7, "username": "project_1_bot", "state": "active"}`),
	})
	sel, _ := gitlabSelectionOf(id)
	if !sel.ServiceIdentity() {
		t.Fatal("precondition: the fixture provider uses a service token")
	}

	acct, err := p.Account(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if acct.Login != "project_1_bot" {
		t.Errorf("login = %q; a GitLab bot token resolves through /user, so there is no reason to "+
			"short-circuit the call the way the GitHub App tier must", acct.Login)
	}
}

func TestGitLabRateLimitComesFromResponseHeaders(t *testing.T) {
	p, id := gitlabAPI(t, map[string]http.HandlerFunc{
		"user": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("RateLimit-Limit", "2000")
			w.Header().Set("RateLimit-Remaining", "1997")
			w.Write([]byte(gitlabUserFixture))
		},
	})

	rl, err := p.RateLimit(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if rl.Limit != 2000 || rl.Remaining != 1997 {
		t.Errorf("rate limit = %d/%d, want 1997/2000 from the headers", rl.Remaining, rl.Limit)
	}
}

func TestGitLabRateLimitErrorsWhenTheHostAdvertisesNone(t *testing.T) {
	p, id := gitlabAPI(t, map[string]http.HandlerFunc{"user": jsonRoute(gitlabUserFixture)})

	_, err := p.RateLimit(context.Background(), id)
	if err == nil {
		t.Fatal("RateLimit returned a zero value with no error; the status strip reads Remaining == 0 " +
			"as an exhausted quota and would permanently red-flag a healthy self-managed instance")
	}
}

func TestGitLabSSHSigningKeyRequiresSigningUsage(t *testing.T) {
	p, id := gitlabAPI(t, map[string]http.HandlerFunc{"user/keys": jsonRoute(gitlabSSHKeysFixture)})

	cases := []struct {
		name string
		key  string
		want bool
	}{
		{"signing key", "ssh-ed25519 AAAAKEYBODY laptop@home", true},
		{"comment is ignored", "ssh-ed25519 AAAAKEYBODY someone@else", true},
		{"auth-only key does not count", "ssh-ed25519 AAAAAUTHONLY box", false},
		{"absent usage_type falls back to a body match", "ssh-ed25519 AAAALEGACY old", true},
		{"unknown key", "ssh-ed25519 AAAANOTTHERE x", false},
		{"malformed key", "not-a-key", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := p.SigningKeyRegistered(context.Background(), id, gitauth.SigningSSH, c.key)
			if got.Err != nil {
				t.Fatal(got.Err)
			}
			if got.Registered != c.want {
				t.Errorf("Registered = %v, want %v", got.Registered, c.want)
			}
		})
	}
}

func TestGitLabEmailVerifiedIncludesThePrimaryFromUser(t *testing.T) {
	p, id := gitlabAPI(t, map[string]http.HandlerFunc{
		"user":        jsonRoute(gitlabUserFixture),
		"user/emails": jsonRoute(gitlabEmailsFixture),
	})

	cases := []struct {
		email string
		want  bool
	}{
		{"cody@example.com", true},
		{"CODY@EXAMPLE.COM", true},
		{"secondary@example.com", true},
		{"unconfirmed@example.com", false},
		{"someone@else.com", false},
	}
	for _, c := range cases {
		got, err := p.EmailVerified(context.Background(), id, c.email)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("EmailVerified(%q) = %v, want %v; the primary address is not in /user/emails on "+
				"every GitLab version, so checking only that list calls a real commit email unverified",
				c.email, got, c.want)
		}
	}
}

func TestGitLabConfirmedEmailsAreFetchedOnce(t *testing.T) {
	var emailCalls int
	p, id := gitlabAPI(t, map[string]http.HandlerFunc{
		"user": jsonRoute(gitlabUserFixture),
		"user/emails": func(w http.ResponseWriter, _ *http.Request) {
			emailCalls++
			w.Write([]byte(gitlabEmailsFixture))
		},
	})

	for range 3 {
		if _, err := p.EmailVerified(context.Background(), id, "cody@example.com"); err != nil {
			t.Fatal(err)
		}
	}
	if emailCalls != 1 {
		t.Errorf("/user/emails fetched %d times, want 1; onboarding hits this from both the GPG and "+
			"the email check", emailCalls)
	}
}

func TestGitLabStatusPinsTheConfiguredHostname(t *testing.T) {
	argsFile := t.TempDir() + "/args"
	fakeGLab(t, "#!/bin/sh\necho \"$@\" > \""+argsFile+"\"\n")

	p := &gitlabProvider{spec: GitLabSpec{APIURL: "https://gl.example.com/api/v4", Store: memStore{}}}
	id, err := p.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	st := p.Status(context.Background(), id)
	if !st.OK || st.Detail != "glab CLI is logged in" {
		t.Fatalf("Status = %+v", st)
	}
	recorded, err := readTrimmed(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if recorded != "auth status --hostname gl.example.com" {
		t.Errorf("glab args = %q, want the configured host pinned; otherwise glab answers about the "+
			"wrong instance", recorded)
	}
}

func TestGitLabStatusExplainsHowToLogIn(t *testing.T) {
	withoutGLabOnPath(t)
	clearAmbientGitLab(t)

	p := &gitlabProvider{spec: GitLabSpec{Store: memStore{}}}
	id, err := p.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	st := p.Status(context.Background(), id)
	if st.OK {
		t.Fatal("Status reported OK with nothing configured")
	}
	if strings.Join(st.Fix, " ") != "glab auth login mino login gitlab" {
		t.Errorf("Fix = %v, want both login routes", st.Fix)
	}
}

func TestGitLabScopeFixNamesReadUserAndTheHost(t *testing.T) {
	p := &gitlabProvider{spec: GitLabSpec{APIURL: "https://gl.example.com/api/v4"}}

	fix := p.scopeFix(errs.New(errs.KindAuth, "gitlab api 403 Forbidden: insufficient_scope"))
	if len(fix) == 0 {
		t.Fatal("a scope failure produced no fix")
	}
	if !strings.Contains(fix[0], gitlabReadScope) || !strings.Contains(fix[0], "gl.example.com") {
		t.Errorf("fix = %q, want the scope and the host", fix[0])
	}

	if got := p.scopeFix(errs.New(errs.KindSignal, "connection refused")); got != nil {
		t.Errorf("an unrelated error produced a scope fix: %v", got)
	}
	if got := p.scopeFix(nil); got != nil {
		t.Errorf("a nil error produced a fix: %v", got)
	}
}

func TestGitLabUploadKeyFixPointsAtUserSettings(t *testing.T) {
	p := &gitlabProvider{spec: GitLabSpec{APIURL: "https://gl.example.com/api/v4"}}

	ssh := strings.Join(p.UploadKeyFix(gitauth.SigningSSH, ""), " ")
	if !strings.Contains(ssh, "glab ssh-key add") || !strings.Contains(ssh, "gl.example.com") {
		t.Errorf("ssh fix = %q", ssh)
	}
	gpg := strings.Join(p.UploadKeyFix(gitauth.SigningGPG, "ABCD1234"), " ")
	if strings.Contains(gpg, "glab gpg-key") {
		t.Error("the GPG fix suggests `glab gpg-key`, which glab does not have")
	}
	if !strings.Contains(gpg, "ABCD1234") {
		t.Errorf("gpg fix = %q, want it to name the key", gpg)
	}
}

func TestGitLabFindingsNameTheSelectedTierAndHost(t *testing.T) {
	p, id := gitlabAPI(t, nil)

	byName := map[string]gitauth.Finding{}
	for _, f := range p.Findings(context.Background(), id) {
		byName[f.Name] = f
	}
	if got := byName["gitlab.auth.selected"]; !got.OK || !strings.Contains(got.Msg, "service identity") {
		t.Errorf("gitlab.auth.selected = %+v, want the service tier named", got)
	}
	if got := byName["gitlab.service_token"]; !got.OK {
		t.Errorf("gitlab.service_token = %+v, want it reported as set", got)
	}
	if got := byName["gitlab.api_url"]; !strings.Contains(got.Msg, "self-managed") {
		t.Errorf("gitlab.api_url = %+v, want the host classified", got)
	}
	if got := byName["gitlab.oauth_client_id"]; !strings.Contains(got.Msg, "unset") {
		t.Errorf("gitlab.oauth_client_id = %+v, want the unset case explained", got)
	}
}

func TestGitLabFindingsWarnAboutASilentlyExpiringToken(t *testing.T) {
	withoutGLabOnPath(t)
	clearAmbientGitLab(t)

	store := memStore{gitlabCredKey: {AccessToken: "at", Expiry: nowPlusHours(2)}}
	p := &gitlabProvider{spec: GitLabSpec{Store: store}}
	id, err := p.Resolve()
	if err != nil {
		t.Fatal(err)
	}

	var found bool
	for _, f := range p.Findings(context.Background(), id) {
		if f.Name != "gitlab.token.expiry" {
			continue
		}
		found = true
		if !f.Warn {
			t.Errorf("gitlab.token.expiry = %+v, want a warning; a cached token with no refresh token "+
				"stops working in two hours and nothing renews it", f)
		}
	}
	if !found {
		t.Error("no gitlab.token.expiry finding; `mino verify auth` is where a silently-expiring " +
			"install has to surface")
	}
}
