package auth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type memStore map[string]Credential

func (m memStore) Get(s string) (Credential, bool, error) { c, ok := m[s]; return c, ok, nil }
func (m memStore) Put(s string, c Credential) error       { m[s] = c; return nil }
func (m memStore) Delete(s string) error                  { delete(m, s); return nil }

func TestGitHubTokenPrecedence(t *testing.T) {
	store := memStore{}

	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	if tok, _ := GitHubToken(store); tok != "" {
		t.Fatalf("expected no token, got %q", tok)
	}

	if err := CacheGitHubToken(store, "cached-tok", "repo"); err != nil {
		t.Fatal(err)
	}
	if tok, origin := GitHubToken(store); tok != "cached-tok" || origin != "cached OAuth token" {
		t.Fatalf("cached lookup = %q/%q", tok, origin)
	}

	t.Setenv("GH_TOKEN", "gh-tok")
	if tok, origin := GitHubToken(store); tok != "gh-tok" || origin != "$GH_TOKEN" {
		t.Fatalf("GH_TOKEN precedence = %q/%q", tok, origin)
	}
	t.Setenv("GITHUB_TOKEN", "github-tok")
	if tok, origin := GitHubToken(store); tok != "github-tok" || origin != "$GITHUB_TOKEN" {
		t.Fatalf("GITHUB_TOKEN precedence = %q/%q", tok, origin)
	}
}

func TestCacheRoundTrip(t *testing.T) {
	store := memStore{}
	if err := CacheGitHubToken(store, "abc", "repo read:org"); err != nil {
		t.Fatal(err)
	}
	if c, ok := getCred(store, "github"); !ok || c.AccessToken != "abc" || c.Scope != "repo read:org" {
		t.Fatalf("round-trip credential = %+v (ok=%v)", c, ok)
	}
}

func TestDeviceFlowSucceeds(t *testing.T) {
	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/device":
			_, _ = io.WriteString(w, `{"device_code":"DEV","user_code":"WXYZ-1234","verification_uri":"https://github.com/login/device","interval":1,"expires_in":60}`)
		case "/token":
			polls++
			if polls < 2 {
				_, _ = io.WriteString(w, `{"error":"authorization_pending"}`)
				return
			}
			_, _ = io.WriteString(w, `{"access_token":"gho_final"}`)
		}
	}))
	defer srv.Close()

	var slept int
	tok, err := runGitHubDeviceFlow(context.Background(), srv.Client(),
		srv.URL+"/device", srv.URL+"/token", "client-id", "repo",
		io.Discard, func(time.Duration) { slept++ })
	if err != nil {
		t.Fatal(err)
	}
	if tok != "gho_final" {
		t.Fatalf("token = %q, want gho_final", tok)
	}
	if polls < 2 {
		t.Errorf("expected polling until authorized, polls=%d", polls)
	}
}

func TestDeviceFlowDenied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/device" {
			_, _ = io.WriteString(w, `{"device_code":"DEV","user_code":"C","verification_uri":"u","interval":1,"expires_in":60}`)
			return
		}
		_, _ = io.WriteString(w, `{"error":"access_denied"}`)
	}))
	defer srv.Close()

	_, err := runGitHubDeviceFlow(context.Background(), srv.Client(),
		srv.URL+"/device", srv.URL+"/token", "client-id", "", io.Discard, func(time.Duration) {})
	if err == nil {
		t.Fatal("expected error when access is denied")
	}
}
