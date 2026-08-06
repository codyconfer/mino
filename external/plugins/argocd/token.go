package argocd

import (
	"context"
	"os"
	"strings"

	"github.com/codyconfer/mino/external/plugins/internal/errx"
	"github.com/codyconfer/mino/plugin"
)

type TokenLookup interface {
	Get(ctx context.Context, service string) (accessToken, scope string, ok bool, err error)
}

type tokenSourceLookup struct{ src plugin.TokenSource }

func (t tokenSourceLookup) Get(ctx context.Context, service string) (string, string, bool, error) {
	if t.src == nil {
		return "", "", false, nil
	}
	return t.src.GetToken(ctx, service)
}

type credentialLookup struct{ store plugin.CredentialStore }

func (c credentialLookup) Get(ctx context.Context, service string) (string, string, bool, error) {
	if c.store == nil {
		return "", "", false, nil
	}
	cred, ok, err := c.store.Get(ctx, service)
	if err != nil || !ok {
		return "", "", false, err
	}
	return cred.AccessToken, cred.Scope, cred.AccessToken != "", nil
}

func tokenLookupFrom(bc plugin.BuildContext) TokenLookup {
	if ts, ok := bc.(plugin.TokenSource); ok && ts != nil {
		return tokenSourceLookup{src: ts}
	}
	if store := plugin.CredentialsOf(bc); store != nil {
		return credentialLookup{store: store}
	}
	return nil
}

func resolveToken(ctx context.Context, lookup TokenLookup, envName string) (string, error) {
	if envName = strings.TrimSpace(envName); envName == "" {
		envName = DefaultTokenEnv
	}
	if lookup != nil {
		tok, _, ok, err := lookup.Get(ctx, TokenKey)
		if err == nil && ok {
			if tok = strings.TrimSpace(tok); tok != "" {
				return tok, nil
			}
		}
	}
	if tok := strings.TrimSpace(os.Getenv(envName)); tok != "" {
		return tok, nil
	}
	return "", errx.New("argocd: no API token available").
		WithHint("seal one under the `argocd` credential key, or export $%s "+
			"(mint it with `argocd account generate-token --account mino`)", envName)
}
