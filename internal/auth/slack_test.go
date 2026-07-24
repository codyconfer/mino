package auth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestSlackTokenPrecedence(t *testing.T) {
	store := memStore{}
	t.Setenv("SLACK_TOKEN", "")

	if _, err := SlackToken(store, "SLACK_TOKEN"); err == nil {
		t.Fatal("expected error with no token available")
	}

	if err := store.Put(context.Background(), "slack", Credential{AccessToken: "xoxp-cached", Scope: "scopes"}); err != nil {
		t.Fatal(err)
	}
	if tok, err := SlackToken(store, "SLACK_TOKEN"); err != nil || tok != "xoxp-cached" {
		t.Fatalf("cached lookup = %q, %v", tok, err)
	}

	t.Setenv("MY_SLACK", "xoxp-env")
	if tok, err := SlackToken(store, "MY_SLACK"); err != nil || tok != "xoxp-env" {
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

	orig := slackTokenURL
	slackTokenURL = srv.URL
	defer func() { slackTokenURL = orig }()

	tok, err := slackExchange(context.Background(), srv.Client(),
		SlackAuth{ClientID: "id", ClientSecret: "secret"}, "auth-code", "http://127.0.0.1:1/callback", "the-verifier")
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
	orig := slackTokenURL
	slackTokenURL = srv.URL
	defer func() { slackTokenURL = orig }()

	if _, err := slackExchange(context.Background(), srv.Client(), SlackAuth{}, "bad", "r", "v"); err == nil {
		t.Fatal("expected error for ok:false response")
	}
}

func TestSlackLoginPKCE(t *testing.T) {
	store := memStore{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"authed_user":{"access_token":"xoxp-fresh"}}`))
	}))
	defer srv.Close()
	origTok := slackTokenURL
	slackTokenURL = srv.URL
	defer func() { slackTokenURL = origTok }()

	var gotURL string
	origBrowser := openBrowser
	openBrowser = func(u string) error {
		gotURL = u
		parsed, _ := url.Parse(u)
		q := parsed.Query()
		go http.Get(q.Get("redirect_uri") + "?code=c&state=" + q.Get("state"))
		return nil
	}
	defer func() { openBrowser = origBrowser }()

	if err := SlackLogin(context.Background(), SlackAuth{Store: store, ClientID: "id", ClientSecret: "sec"}, io.Discard); err != nil {
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
	v, err := slackVerifier()
	if err != nil {
		t.Fatal(err)
	}
	if len(v) < 43 || len(v) > 128 {
		t.Errorf("verifier length = %d, want 43..128", len(v))
	}
	c := slackChallenge(v)
	if c == "" || c == v {
		t.Errorf("challenge = %q", c)
	}
}
