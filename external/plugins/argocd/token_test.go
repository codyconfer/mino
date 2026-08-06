package argocd

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSealedStoreBeatsTheEnvironment(t *testing.T) {
	t.Setenv(DefaultTokenEnv, "from-env")

	got, err := resolveToken(context.Background(), staticTokens{token: "from-store"}, DefaultTokenEnv)
	if err != nil {
		t.Fatalf("resolveToken: %v", err)
	}
	if got != "from-store" {
		t.Errorf("resolveToken = %q, want the sealed credential; a stale $%s from a shell profile must not "+
			"silently point mino at another server", got, DefaultTokenEnv)
	}
}

func TestEnvironmentUsedWhenTheStoreIsEmpty(t *testing.T) {
	t.Setenv(DefaultTokenEnv, "from-env")

	got, err := resolveToken(context.Background(), staticTokens{}, DefaultTokenEnv)
	if err != nil {
		t.Fatalf("resolveToken: %v", err)
	}
	if got != "from-env" {
		t.Errorf("resolveToken = %q, want the env fallback; there is no `mino token set`, so the env var "+
			"is the only supported path today", got)
	}
}

func TestStoreErrorFallsThroughToTheEnvironment(t *testing.T) {
	t.Setenv(DefaultTokenEnv, "from-env")

	got, err := resolveToken(context.Background(), staticTokens{err: errors.New("sealed store unreadable")}, DefaultTokenEnv)
	if err != nil {
		t.Fatalf("resolveToken: %v", err)
	}
	if got != "from-env" {
		t.Errorf("resolveToken = %q; an unreadable credential store must not strand a user who has the "+
			"env var set", got)
	}
}

func TestCustomTokenEnvIsHonoured(t *testing.T) {
	t.Setenv("MY_ARGOCD_TOKEN", "custom")
	t.Setenv(DefaultTokenEnv, "default")

	got, err := resolveToken(context.Background(), nil, "MY_ARGOCD_TOKEN")
	if err != nil {
		t.Fatalf("resolveToken: %v", err)
	}
	if got != "custom" {
		t.Errorf("resolveToken = %q, want the token_env override to be read", got)
	}
}

func TestMissingTokenNamesTheConfiguredEnvVar(t *testing.T) {
	t.Setenv("MY_ARGOCD_TOKEN", "")

	_, err := resolveToken(context.Background(), nil, "MY_ARGOCD_TOKEN")
	if err == nil {
		t.Fatal("resolveToken succeeded with no credential anywhere")
	}
	if !strings.Contains(err.Error(), "MY_ARGOCD_TOKEN") {
		t.Errorf("error = %q, want it to name the configured token_env rather than the default, or the "+
			"user looks at the wrong variable", err)
	}
	if !strings.Contains(err.Error(), "generate-token") {
		t.Errorf("error = %q, want the hint to say how to mint a token", err)
	}
}

func TestNilLookupIsTolerated(t *testing.T) {
	t.Setenv(DefaultTokenEnv, "from-env")

	got, err := resolveToken(context.Background(), nil, "")
	if err != nil {
		t.Fatalf("resolveToken with a nil lookup: %v", err)
	}
	if got != "from-env" {
		t.Errorf("resolveToken = %q, want the env fallback when no store is wired in", got)
	}
}

func TestBlankStoreTokenFallsThrough(t *testing.T) {
	t.Setenv(DefaultTokenEnv, "from-env")

	got, err := resolveToken(context.Background(), staticTokens{token: "   "}, DefaultTokenEnv)
	if err != nil {
		t.Fatalf("resolveToken: %v", err)
	}
	if got != "from-env" {
		t.Errorf("resolveToken = %q, want a whitespace-only sealed value treated as absent", got)
	}
}

func TestTokenLookupFromPrefersTheTokenSource(t *testing.T) {
	lookup := tokenLookupFrom(buildCtx{token: "from-source"})
	if lookup == nil {
		t.Fatal("tokenLookupFrom returned nil for a BuildContext implementing plugin.TokenSource")
	}
	got, _, ok, err := lookup.Get(context.Background(), TokenKey)
	if err != nil || !ok || got != "from-source" {
		t.Fatalf("Get = %q ok=%v err=%v, want the TokenSource value", got, ok, err)
	}
}
