package auth

import (
	"context"
	"io"

	"github.com/codyconfer/mino/internal/errs"
)

const defaultGitHubOAuthScope = "repo read:org"

const defaultGitLabOAuthScope = "read_api read_user"

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

func LoginGitLab(ctx context.Context, store TokenStore, apiURL, clientID, scopes string, w io.Writer) error {
	if clientID == "" {
		return errs.New(errs.KindConfig, "gitlab.oauth_client_id is not set").
			WithHint("set `gitlab.oauth_client_id` in config.yaml to a GitLab OAuth application id " +
				"(the application must not be marked Confidential) to use device-flow login")
	}
	if scopes == "" {
		scopes = defaultGitLabOAuthScope
	}
	cred, err := GitLabDeviceFlow(ctx, apiURL, clientID, scopes, w)
	if err != nil {
		return err
	}
	if cred.Scope == "" {
		cred.Scope = scopes
	}
	if err := CacheGitLabCredential(store, cred); err != nil {
		return errs.Wrap(errs.KindAuth, err, "caching token")
	}
	return nil
}
