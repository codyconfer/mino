package slack

import (
	"context"
	"io"

	"github.com/codyconfer/mino/external/plugins/internal/slackauth"
	"github.com/codyconfer/mino/plugin"
)

func login(ctx context.Context, h plugin.Host, creds map[string]string, w io.Writer) error {
	cfg := slackauth.FromHost(h)
	return slackauth.Login(ctx, slackauth.Auth{
		Store:        cfg.Store,
		ClientID:     effective(creds, "plugins.slack.oauth_client_id", cfg.OAuthClientID),
		ClientSecret: effective(creds, "plugins.slack.oauth_client_secret", cfg.OAuthClientSecret),
		UserScopes:   cfg.UserScopes,
	}, w)
}

func effective(creds map[string]string, key, current string) string {
	if v := creds[key]; v != "" {
		return v
	}
	return current
}
