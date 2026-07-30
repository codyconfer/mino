package auth

import (
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestGoogleTokenCacheRoundTrip(t *testing.T) {
	store := memStore{}

	if tok := readGoogleToken(store); tok != nil {
		t.Fatal("expected no cached token initially")
	}

	want := &oauth2.Token{
		AccessToken:  "ya29.access",
		RefreshToken: "1//refresh",
		Expiry:       time.Now().Add(time.Hour),
	}
	if err := cacheGoogleToken(store, want); err != nil {
		t.Fatal(err)
	}
	got := readGoogleToken(store)
	if got == nil || got.AccessToken != "ya29.access" || got.RefreshToken != "1//refresh" {
		t.Fatalf("round-trip token = %+v", got)
	}
}

func TestGoogleTokenSourceKind(t *testing.T) {
	tok := &oauth2.Token{AccessToken: "a", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)}

	ts := googleTokenSource(t.Context(), GoogleAuth{}, GoogleLoginScopes, tok)
	got, err := ts.Token()
	if err != nil || got.AccessToken != "a" {
		t.Fatalf("static token source = %+v, %v", got, err)
	}

	ts2 := googleTokenSource(t.Context(), GoogleAuth{ClientID: "id", ClientSecret: "sec"}, GoogleLoginScopes, tok)
	if got, err := ts2.Token(); err != nil || got.AccessToken != "a" {
		t.Fatalf("refreshing token source (valid token) = %+v, %v", got, err)
	}
}

func TestMissingScopesUsesTheMemoisedVerdict(t *testing.T) {
	grantedScopes.mu.Lock()
	grantedScopes.m = map[string]map[string]bool{
		"tok": {"https://www.googleapis.com/auth/tasks": true},
	}
	grantedScopes.mu.Unlock()
	t.Cleanup(func() {
		grantedScopes.mu.Lock()
		grantedScopes.m = nil
		grantedScopes.mu.Unlock()
	})

	if got := missingScopes(t.Context(), "tok", []string{"https://www.googleapis.com/auth/tasks"}); len(got) != 0 {
		t.Fatalf("missingScopes = %v, want none for a granted scope", got)
	}
	want := "https://www.googleapis.com/auth/gmail.readonly"
	got := missingScopes(t.Context(), "tok", []string{want, "openid", "email"})
	if len(got) != 1 || got[0] != want {
		t.Fatalf("missingScopes = %v, want just %s (openid/email need no grant)", got, want)
	}
}

func TestGoogleLoginScopesDoNotRequestFullDrive(t *testing.T) {
	for _, s := range GoogleLoginScopes {
		if s == "https://www.googleapis.com/auth/drive" {
			t.Fatal("GoogleLoginScopes requests full read/write Drive; munin only lists metadata and writes its own files")
		}
	}
}

func TestPersistingGoogleTokenSource(t *testing.T) {
	store := memStore{}
	refreshed := &oauth2.Token{AccessToken: "b", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)}
	p := &persistingGoogleTokenSource{
		store: store,
		src:   oauth2.StaticTokenSource(refreshed),
		last:  "a",
	}
	got, err := p.Token()
	if err != nil || got.AccessToken != "b" {
		t.Fatalf("token = %+v, %v", got, err)
	}
	cached := readGoogleToken(store)
	if cached == nil || cached.AccessToken != "b" {
		t.Fatalf("refreshed token not persisted: %+v", cached)
	}
}
