package auth

import (
	"context"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/errs"
)

func clearAmbientGitea(t *testing.T) {
	t.Helper()
	t.Setenv("GITEA_TOKEN", "")
	t.Setenv("FORGEJO_TOKEN", "")
}

func giteaSpec(store TokenStore) GiteaSpec {
	return GiteaSpec{Forge: "gitea", URL: "https://git.example.com", Store: store}
}

func TestGiteaAPIBaseAppendsAPIV1(t *testing.T) {
	cases := []struct {
		name    string
		spec    GiteaSpec
		wantAPI string
		wantWeb string
	}{
		{"root", GiteaSpec{URL: "https://git.example.com"}, "https://git.example.com/api/v1", "https://git.example.com"},
		{"root with slash", GiteaSpec{URL: "https://git.example.com/"}, "https://git.example.com/api/v1", "https://git.example.com"},
		{"subpath install", GiteaSpec{URL: "https://example.com/gitea"}, "https://example.com/gitea/api/v1", "https://example.com/gitea"},
		{"api_url wins", GiteaSpec{URL: "https://a.example.com", APIURL: "https://b.example.com/api/v1"}, "https://b.example.com/api/v1", "https://a.example.com"},
		{"api_url only", GiteaSpec{APIURL: "https://b.example.com/api/v1"}, "https://b.example.com/api/v1", "https://b.example.com"},
		{"nothing", GiteaSpec{}, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.spec.APIBase(); got != c.wantAPI {
				t.Errorf("APIBase() = %q, want %q", got, c.wantAPI)
			}
			if got := c.spec.WebBase(); got != c.wantWeb {
				t.Errorf("WebBase() = %q, want %q; the web root is what key-upload hints link to", got, c.wantWeb)
			}
		})
	}
}

func TestSelectGiteaPrefersServiceAuthOverEverythingAmbient(t *testing.T) {
	t.Setenv("GITEA_TOKEN", "env-tok")
	store := memStore{
		giteaCredKey:        {AccessToken: "stored-tok"},
		giteaServiceCredKey: {AccessToken: "sealed-tok"},
	}
	spec := giteaSpec(store)
	spec.ServiceToken = "cfg-tok"

	sel, err := SelectGitea(spec)
	if err != nil {
		t.Fatal(err)
	}
	if sel.Mech != GiteaServiceToken || sel.Origin != "gitea.service_token" {
		t.Fatalf("selected %v/%q, want the configured service token to outrank everything ambient", sel.Mech, sel.Origin)
	}
	if !sel.ServiceIdentity() {
		t.Error("a configured service token must report a service identity")
	}
	tok, err := sel.Token(context.Background())
	if err != nil || tok != "cfg-tok" {
		t.Fatalf("Token() = %q/%v", tok, err)
	}
}

func TestSelectGiteaFallsBackThroughTheAmbientTiers(t *testing.T) {
	cases := []struct {
		name       string
		gitea      string
		forgejo    string
		store      memStore
		wantMech   GiteaMechanism
		wantOrigin string
		wantToken  string
	}{
		{
			name: "sealed service key", store: memStore{giteaServiceCredKey: {AccessToken: "sealed-tok"}},
			wantMech: GiteaServiceToken, wantOrigin: `sealed store "gitea-service"`, wantToken: "sealed-tok",
		},
		{
			name: "both env vars", gitea: "g-tok", forgejo: "f-tok", store: memStore{},
			wantMech: GiteaEnvToken, wantOrigin: "$GITEA_TOKEN", wantToken: "g-tok",
		},
		{
			name: "forgejo env only", forgejo: "f-tok", store: memStore{},
			wantMech: GiteaEnvToken, wantOrigin: "$FORGEJO_TOKEN", wantToken: "f-tok",
		},
		{
			name: "stored login", store: memStore{giteaCredKey: {AccessToken: "stored-tok"}},
			wantMech: GiteaStoredToken, wantOrigin: "cached personal access token", wantToken: "stored-tok",
		},
		{
			name: "nothing", store: memStore{},
			wantMech: GiteaNone, wantOrigin: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			clearAmbientGitea(t)
			if c.gitea != "" {
				t.Setenv("GITEA_TOKEN", c.gitea)
			}
			if c.forgejo != "" {
				t.Setenv("FORGEJO_TOKEN", c.forgejo)
			}
			sel, err := SelectGitea(giteaSpec(c.store))
			if err != nil {
				t.Fatal(err)
			}
			if sel.Mech != c.wantMech || sel.Origin != c.wantOrigin {
				t.Fatalf("selected %v/%q, want %v/%q", sel.Mech, sel.Origin, c.wantMech, c.wantOrigin)
			}
			if c.wantToken == "" {
				return
			}
			if tok, err := sel.Token(context.Background()); err != nil || tok != c.wantToken {
				t.Fatalf("Token() = %q/%v, want %q", tok, err, c.wantToken)
			}
		})
	}
}

func TestSelectGiteaTraceNamesEveryTierAndTheWinner(t *testing.T) {
	clearAmbientGitea(t)
	t.Setenv("GITEA_TOKEN", "env-tok")

	sel, err := SelectGitea(giteaSpec(memStore{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"gitea: auth tiers:",
		"service_token=unset",
		"store:gitea-service=miss",
		"ambient=$GITEA_TOKEN",
		"-> selected env token ($GITEA_TOKEN)",
	} {
		if !strings.Contains(sel.Trace(), want) {
			t.Errorf("trace %q is missing %q; `mino verify auth -v` shows which tiers lost and why", sel.Trace(), want)
		}
	}
}

func TestSelectGiteaTraceEndsWithNoneWhenNothingResolves(t *testing.T) {
	clearAmbientGitea(t)

	sel, err := SelectGitea(giteaSpec(memStore{}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(sel.Trace(), "-> none") {
		t.Errorf("trace = %q, want it to end in -> none", sel.Trace())
	}
}

func TestGiteaTracePrefixUsesTheRegisteredForgeName(t *testing.T) {
	clearAmbientGitea(t)
	spec := giteaSpec(memStore{})
	spec.Forge = "forgejo"

	sel, err := SelectGitea(spec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sel.Trace(), "forgejo: auth tiers:") {
		t.Errorf("trace = %q, want the forgejo prefix so the trace names the provider the user configured", sel.Trace())
	}
}

func TestForgejoSharesGiteasStoreKeys(t *testing.T) {
	clearAmbientGitea(t)
	store := memStore{}
	if err := CacheGiteaToken(store, "sealed-by-login"); err != nil {
		t.Fatal(err)
	}
	spec := giteaSpec(store)
	spec.Forge = "forgejo"

	sel, err := SelectGitea(spec)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := sel.Token(context.Background())
	if err != nil || tok != "sealed-by-login" {
		t.Fatalf("Token() = %q/%v; `mino login gitea` and git.provider forgejo must share one credential", tok, err)
	}
}

func TestGiteaOriginNeverContainsTheToken(t *testing.T) {
	clearAmbientGitea(t)
	const secret = "gta_supersecrettoken"
	spec := giteaSpec(memStore{})
	spec.ServiceToken = secret

	sel, err := SelectGitea(spec)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sel.Origin, secret) || strings.Contains(sel.Trace(), secret) {
		t.Error("the token leaked into the origin or trace, both of which are printed by mino verify auth")
	}
	if got := GiteaTokenOrigin(memStore{giteaCredKey: {AccessToken: secret}}); strings.Contains(got, secret) {
		t.Errorf("GiteaTokenOrigin returned %q; login menus render this string", got)
	}
}

func TestSelectGiteaUnauthenticatedTokenExplainsEveryOption(t *testing.T) {
	clearAmbientGitea(t)

	sel, err := SelectGitea(giteaSpec(memStore{}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = sel.Token(context.Background())
	if err == nil {
		t.Fatal("Token() succeeded with no credential")
	}
	for _, want := range []string{"mino login gitea", "$GITEA_TOKEN", "gitea.service_token"} {
		if !strings.Contains(err.Error()+" "+errs.Hint(err), want) {
			t.Errorf("error %q / hint %q is missing %q", err, errs.Hint(err), want)
		}
	}
}
