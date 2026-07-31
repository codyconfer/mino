package slackauth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/codyconfer/munin/external/plugins/internal/httpx"
	"github.com/codyconfer/munin/plugin"
)

func TestUserTokenPrecedence(t *testing.T) {
	store := memStore{}
	t.Setenv("SLACK_TOKEN", "")

	if _, err := UserToken(store, "SLACK_TOKEN"); err == nil {
		t.Fatal("expected error with no token available")
	}

	if err := store.Put(context.Background(), "slack", plugin.Credential{AccessToken: "xoxp-cached", Scope: "scopes"}); err != nil {
		t.Fatal(err)
	}
	if tok, err := UserToken(store, "SLACK_TOKEN"); err != nil || tok != "xoxp-cached" {
		t.Fatalf("cached lookup = %q, %v", tok, err)
	}

	t.Setenv("MY_SLACK", "xoxp-env")
	if tok, err := UserToken(store, "MY_SLACK"); err != nil || tok != "xoxp-env" {
		t.Fatalf("env lookup = %q, %v", tok, err)
	}
}

func TestSlackExchange(t *testing.T) {
	var gotForm, gotVerifier string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.Form.Get("code")
		gotVerifier = r.Form.Get("code_verifier")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"authed_user":{"access_token":"xoxp-fresh"}}`))
	}))
	defer srv.Close()

	orig := tokenURL
	tokenURL = srv.URL
	defer func() { tokenURL = orig }()

	tok, err := exchange(context.Background(), srv.Client(),
		Auth{ClientID: "id", ClientSecret: "secret"}, "auth-code", "http://127.0.0.1:1/callback", "the-verifier")
	if err != nil {
		t.Fatal(err)
	}
	if tok != "xoxp-fresh" {
		t.Errorf("token = %q, want xoxp-fresh", tok)
	}
	if gotForm != "auth-code" {
		t.Errorf("exchange sent code %q", gotForm)
	}
	if gotVerifier != "the-verifier" {
		t.Errorf("exchange sent code_verifier %q", gotVerifier)
	}
}

func TestSlackExchangeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":false,"error":"invalid_code"}`))
	}))
	defer srv.Close()
	orig := tokenURL
	tokenURL = srv.URL
	defer func() { tokenURL = orig }()

	if _, err := exchange(context.Background(), srv.Client(), Auth{}, "bad", "r", "v"); err == nil {
		t.Fatal("expected error for ok:false response")
	}
}

func TestLoginPKCE(t *testing.T) {
	store := memStore{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"authed_user":{"access_token":"xoxp-fresh"}}`))
	}))
	defer srv.Close()
	origTok := tokenURL
	tokenURL = srv.URL
	defer func() { tokenURL = origTok }()

	var gotURL string
	origBrowser := httpx.OpenBrowser
	httpx.OpenBrowser = func(u string) error {
		gotURL = u
		parsed, _ := url.Parse(u)
		q := parsed.Query()
		go http.Get(q.Get("redirect_uri") + "?code=c&state=" + q.Get("state"))
		return nil
	}
	defer func() { httpx.OpenBrowser = origBrowser }()

	if err := Login(context.Background(), Auth{Store: store, ClientID: "id", ClientSecret: "sec"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(gotURL)
	q := parsed.Query()
	if q.Get("code_challenge_method") != "S256" {
		t.Errorf("authorize URL missing S256, got %q", gotURL)
	}
	if q.Get("code_challenge") == "" {
		t.Error("authorize URL missing code_challenge")
	}
}

func TestSlackChallenge(t *testing.T) {
	v, err := verifier()
	if err != nil {
		t.Fatal(err)
	}
	if len(v) < 43 || len(v) > 128 {
		t.Errorf("verifier length = %d, want 43..128", len(v))
	}
	c := challenge(v)
	if c == "" || c == v {
		t.Errorf("challenge = %q", c)
	}
}

type memStore map[string]plugin.Credential

func (m memStore) Get(_ context.Context, s string) (plugin.Credential, bool, error) {
	c, ok := m[s]
	return c, ok, nil
}

func (m memStore) Put(_ context.Context, s string, c plugin.Credential) error { m[s] = c; return nil }

func (m memStore) Delete(_ context.Context, s string) error { delete(m, s); return nil }
