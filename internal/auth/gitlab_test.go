package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/errs"
)

func gitlabDeviceServer(t *testing.T, start map[string]any, tokenBodies []map[string]any) (deviceURL, tokenURL string) {
	t.Helper()
	var polls int
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/authorize_device", func(w http.ResponseWriter, _ *http.Request) {
		if err := json.NewEncoder(w).Encode(start); err != nil {
			t.Error(err)
		}
	})
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, _ *http.Request) {
		body := tokenBodies[min(polls, len(tokenBodies)-1)]
		polls++
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Error(err)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL + "/oauth/authorize_device", srv.URL + "/oauth/token"
}

func noSleep(time.Duration) {}

var deviceStartOK = map[string]any{
	"device_code": "dc-1", "user_code": "ABCD-EFGH",
	"verification_uri": "https://gitlab.example.com/oauth/device",
	"expires_in":       600, "interval": 5,
}

func TestGitLabDeviceFlowKeepsTheRefreshTokenAndExpiry(t *testing.T) {
	deviceURL, tokenURL := gitlabDeviceServer(t, deviceStartOK, []map[string]any{{
		"access_token": "at-1", "refresh_token": "rt-1", "expires_in": 7200, "scope": "read_api read_user",
	}})

	before := time.Now()
	cred, err := runGitLabDeviceFlow(context.Background(), deviceURL, tokenURL, "cid", "read_api", io.Discard, noSleep)
	if err != nil {
		t.Fatal(err)
	}
	if cred.AccessToken != "at-1" {
		t.Errorf("access token = %q", cred.AccessToken)
	}
	if cred.RefreshToken != "rt-1" {
		t.Errorf("refresh token = %q, want it kept; without it a GitLab login lasts two hours", cred.RefreshToken)
	}
	if cred.Scope != "read_api read_user" {
		t.Errorf("scope = %q, want what the instance granted", cred.Scope)
	}
	if cred.Expiry.Before(before.Add(time.Hour)) {
		t.Errorf("expiry = %v, want roughly two hours out; dropping expires_in means the refresh "+
			"never fires", cred.Expiry)
	}
}

func TestGitLabDeviceFlowPollsThroughAuthorizationPending(t *testing.T) {
	deviceURL, tokenURL := gitlabDeviceServer(t, deviceStartOK, []map[string]any{
		{"error": "authorization_pending"},
		{"error": "authorization_pending"},
		{"access_token": "at-1", "expires_in": 7200},
	})

	cred, err := runGitLabDeviceFlow(context.Background(), deviceURL, tokenURL, "cid", "", io.Discard, noSleep)
	if err != nil {
		t.Fatal(err)
	}
	if cred.AccessToken != "at-1" {
		t.Errorf("access token = %q, want the flow to keep polling until authorization lands", cred.AccessToken)
	}
}

func TestGitLabDeviceFlowBacksOffOnSlowDown(t *testing.T) {
	deviceURL, tokenURL := gitlabDeviceServer(t, deviceStartOK, []map[string]any{
		{"error": "slow_down"},
		{"access_token": "at-1"},
	})

	var slept []time.Duration
	_, err := runGitLabDeviceFlow(context.Background(), deviceURL, tokenURL, "cid", "", io.Discard,
		func(d time.Duration) { slept = append(slept, d) })
	if err != nil {
		t.Fatal(err)
	}
	if len(slept) != 2 || slept[1] <= slept[0] {
		t.Errorf("sleeps = %v, want the interval to grow after slow_down", slept)
	}
}

func TestGitLabDeviceFlowNamesAConfidentialApplication(t *testing.T) {
	deviceURL, tokenURL := gitlabDeviceServer(t,
		map[string]any{"error": "invalid_client", "error_description": "client authentication failed"},
		[]map[string]any{{}})

	_, err := runGitLabDeviceFlow(context.Background(), deviceURL, tokenURL, "cid", "", io.Discard, noSleep)
	if err == nil {
		t.Fatal("a rejected client id was accepted")
	}
	if errs.KindOf(err) != errs.KindConfig {
		t.Errorf("kind = %v, want config", errs.KindOf(err))
	}
	if !strings.Contains(errs.Hint(err), "Confidential") {
		t.Errorf("hint = %q; a Confidential application is the first thing GitLab device flow trips "+
			"over, so the fix has to be named", errs.Hint(err))
	}
}

func TestGitLabDeviceFlowReportsDenialAndExpiry(t *testing.T) {
	for _, c := range []struct{ code, want string }{
		{"access_denied", "denied"},
		{"expired_token", "expired"},
	} {
		deviceURL, tokenURL := gitlabDeviceServer(t, deviceStartOK, []map[string]any{{"error": c.code}})
		_, err := runGitLabDeviceFlow(context.Background(), deviceURL, tokenURL, "cid", "", io.Discard, noSleep)
		if err == nil {
			t.Fatalf("%s was accepted", c.code)
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s error = %v, want it to say %q", c.code, err, c.want)
		}
	}
}

func TestGitLabDeviceFlowRequiresAClientID(t *testing.T) {
	_, err := runGitLabDeviceFlow(context.Background(), "", "", "  ", "", io.Discard, noSleep)
	if err == nil {
		t.Fatal("an empty client id was accepted")
	}
	if !strings.Contains(errs.Hint(err), "gitlab.oauth_client_id") {
		t.Errorf("hint = %q, want it to name the setting", errs.Hint(err))
	}
}

func TestGitLabOAuthEndpointsDeriveFromTheAPIURL(t *testing.T) {
	cases := []struct {
		in         string
		wantDevice string
		wantToken  string
	}{
		{"", "https://gitlab.com/oauth/authorize_device", "https://gitlab.com/oauth/token"},
		{
			"https://gitlab.example.com/api/v4",
			"https://gitlab.example.com/oauth/authorize_device",
			"https://gitlab.example.com/oauth/token",
		},
		{
			"https://gitlab.example.com",
			"https://gitlab.example.com/oauth/authorize_device",
			"https://gitlab.example.com/oauth/token",
		},
	}
	for _, c := range cases {
		device, token, err := GitLabOAuthEndpoints(c.in)
		if err != nil {
			t.Fatalf("GitLabOAuthEndpoints(%q): %v", c.in, err)
		}
		if device != c.wantDevice || token != c.wantToken {
			t.Errorf("GitLabOAuthEndpoints(%q) = %q, %q; want %q, %q; the OAuth endpoints live at the "+
				"instance root, not under /api/v4", c.in, device, token, c.wantDevice, c.wantToken)
		}
	}
}

func TestGitLabOAuthEndpointsRefuseNonHTTPS(t *testing.T) {
	for _, in := range []string{"http://gitlab.example.com/api/v4", "https://", "://bad"} {
		if _, _, err := GitLabOAuthEndpoints(in); err == nil {
			t.Errorf("GitLabOAuthEndpoints(%q) accepted an endpoint that would leak the device code "+
				"and access token", in)
		}
	}
}

func TestLoginGitLabCachesTheWholeCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case gitlabDevicePath:
			json.NewEncoder(w).Encode(deviceStartOK)
		case gitlabTokenPath:
			json.NewEncoder(w).Encode(map[string]any{
				"access_token": "at-1", "refresh_token": "rt-1", "expires_in": 7200,
			})
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	store := memStore{}
	cred, err := runGitLabDeviceFlow(context.Background(), srv.URL+gitlabDevicePath, srv.URL+gitlabTokenPath,
		"cid", "read_api read_user", io.Discard, noSleep)
	if err != nil {
		t.Fatal(err)
	}
	if cred.Scope == "" {
		cred.Scope = "read_api read_user"
	}
	if err := CacheGitLabCredential(store, cred); err != nil {
		t.Fatal(err)
	}
	got := store[gitlabCredKey]
	if got.AccessToken != "at-1" || got.RefreshToken != "rt-1" || got.Expiry.IsZero() {
		t.Errorf("cached credential = %+v, want access token, refresh token and expiry all kept", got)
	}
}

func TestLoginGitLabRequiresAClientID(t *testing.T) {
	err := LoginGitLab(context.Background(), memStore{}, "", "", "", io.Discard)
	if err == nil {
		t.Fatal("LoginGitLab ran with no client id")
	}
	if errs.KindOf(err) != errs.KindConfig {
		t.Errorf("kind = %v, want config", errs.KindOf(err))
	}
}

func TestGitLabTokenPrecedence(t *testing.T) {
	store := memStore{}
	clearAmbientGitLab(t)
	if tok, _ := GitLabToken(store); tok != "" {
		t.Fatalf("token = %q with nothing set", tok)
	}

	if err := CacheGitLabCredential(store, Credential{AccessToken: "cached"}); err != nil {
		t.Fatal(err)
	}
	if tok, origin := GitLabToken(store); tok != "cached" || origin != originGitLabStored {
		t.Errorf("stored lookup = %q/%q", tok, origin)
	}

	t.Setenv("GL_TOKEN", "gl")
	if _, origin := GitLabToken(store); origin != originGLToken {
		t.Errorf("origin = %q, want $GL_TOKEN", origin)
	}

	t.Setenv("GITLAB_TOKEN", "glpat")
	if tok, origin := GitLabToken(store); tok != "glpat" || origin != originGitLabToken {
		t.Errorf("lookup = %q/%q, want $GITLAB_TOKEN to win", tok, origin)
	}
}

func TestGitLabTokenIgnoresCIJobToken(t *testing.T) {
	clearAmbientGitLab(t)
	t.Setenv("CI_JOB_TOKEN", "job-token")

	if tok, origin := GitLabToken(memStore{}); tok != "" {
		t.Errorf("token = %q from %q; a job token needs the JOB-TOKEN header and cannot read /user, "+
			"so selecting it would fail every provider check for a reason the user cannot act on",
			tok, origin)
	}
}
