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
