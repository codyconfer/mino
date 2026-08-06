package auth

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/errs"
)

func TestLoginGiteaVerifiesTheTokenBeforeSealingIt(t *testing.T) {
	srv := newGiteaServer(t, map[string]func(http.ResponseWriter){
		"/api/v1/user": status(http.StatusUnauthorized, `{"message":"token is invalid"}`),
	})
	store := memStore{}

	err := LoginGitea(context.Background(), store, GiteaSpec{Forge: "gitea", URL: srv.URL, Store: store}, "typo-tok", &bytes.Buffer{})
	if err == nil {
		t.Fatal("a rejected token was accepted")
	}
	if errs.KindOf(err) != errs.KindAuth {
		t.Errorf("kind = %v, want %v", errs.KindOf(err), errs.KindAuth)
	}
	if _, ok := store[giteaCredKey]; ok {
		t.Error("a token the instance rejected was sealed anyway; it would then outrank every ambient tier")
	}
}

func TestLoginGiteaSealsTheTokenUnderTheGiteaKey(t *testing.T) {
	srv := newGiteaServer(t, map[string]func(http.ResponseWriter){
		"/api/v1/user": json200(`{"login":"alice"}`),
	})
	store := memStore{}
	var out bytes.Buffer

	if err := LoginGitea(context.Background(), store, GiteaSpec{Forge: "gitea", URL: srv.URL, Store: store}, " good-tok\n", &out); err != nil {
		t.Fatalf("LoginGitea: %v", err)
	}
	if got := store[giteaCredKey].AccessToken; got != "good-tok" {
		t.Errorf("sealed token = %q, want the trimmed paste", got)
	}
	if !strings.Contains(out.String(), "alice") {
		t.Errorf("output = %q, want the resolved login so the user can see whose token they pasted", out.String())
	}
}

func TestLoginGiteaRefusesAnEmptyTokenAndAMissingURL(t *testing.T) {
	store := memStore{}

	err := LoginGitea(context.Background(), store, GiteaSpec{Forge: "gitea", URL: "https://git.example.com", Store: store}, "  ", &bytes.Buffer{})
	if errs.KindOf(err) != errs.KindUsage {
		t.Errorf("empty token kind = %v, want %v", errs.KindOf(err), errs.KindUsage)
	}
	if hint := errs.Hint(err); !strings.Contains(hint, "/user/settings/applications") {
		t.Errorf("hint = %q, want the page that issues tokens", hint)
	}

	err = LoginGitea(context.Background(), store, GiteaSpec{Forge: "gitea", Store: store}, "tok", &bytes.Buffer{})
	if errs.KindOf(err) != errs.KindConfig {
		t.Errorf("missing url kind = %v, want %v", errs.KindOf(err), errs.KindConfig)
	}
}

func TestGiteaAuthedAsksNoNetwork(t *testing.T) {
	srv := newGiteaServer(t, nil)
	clearAmbientGitea(t)
	store := memStore{}
	spec := GiteaSpec{Forge: "gitea", URL: srv.URL, Store: store}

	if GiteaAuthed(spec) {
		t.Error("GiteaAuthed reported authed with no credential")
	}
	if err := CacheGiteaToken(store, "tok"); err != nil {
		t.Fatal(err)
	}
	if !GiteaAuthed(spec) {
		t.Error("GiteaAuthed ignored a sealed credential")
	}
	if GiteaAuthed(GiteaSpec{Forge: "gitea", Store: store}) {
		t.Error("GiteaAuthed reported authed with no instance URL to reach")
	}
	if got := srv.requested(); len(got) != 0 {
		t.Errorf("GiteaAuthed issued %v; the accounts menu calls it on every render", got)
	}
}
