package auth

import (
	"context"
	"io"

	"github.com/codyconfer/munin/internal/errs"
)

const defaultGitHubOAuthScope = "repo read:org"

func LoginSlack(ctx context.Context, store TokenStore, clientID, clientSecret, userScopes string, w io.Writer) error {
	sa := SlackAuth{
		Store:        store,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		UserScopes:   userScopes,
	}
	return SlackLogin(ctx, sa, w)
}

func LoginGitHub(ctx context.Context, store TokenStore, clientID, scopes string, w io.Writer) error {
	if clientID == "" {
		return errs.New(errs.KindConfig, "github.oauth_client_id is not set").
			WithHint("set `github.oauth_client_id` in config.yaml (a GitHub OAuth App client id) to use device-flow login")
	}
	if scopes == "" {
		scopes = defaultGitHubOAuthScope
	}
	token, err := GitHubDeviceFlow(ctx, clientID, scopes, w)
	if err != nil {
		return err
	}
	if err := CacheGitHubToken(store, token, scopes); err != nil {
		return errs.Wrap(errs.KindAuth, err, "caching token")
	}
	return nil
}
