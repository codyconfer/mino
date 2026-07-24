package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/codyconfer/munin/internal/errs"
)

const DefaultSlackUserScopes = "channels:history,channels:read,groups:history,groups:read"

var (
	slackAuthorizeURL = "https://slack.com/oauth/v2/authorize"
	slackTokenURL     = "https://slack.com/api/oauth.v2.access"
)

type SlackAuth struct {
	Store        TokenStore
	ClientID     string
	ClientSecret string
	UserScopes   string
}

func slackTokenFor(store TokenStore, envName, defaultEnv, credKey, notAvailMsg, hint string) (string, error) {
	if envName == "" {
		envName = defaultEnv
	}
	if tok := os.Getenv(envName); tok != "" {
		return tok, nil
	}
	if c, ok := getCred(store, credKey); ok {
		return c.AccessToken, nil
	}
	return "", errs.New(errs.KindAuth, notAvailMsg).WithHint(hint, envName)
}

func SlackToken(store TokenStore, envName string) (string, error) {
	return slackTokenFor(store, envName, "SLACK_TOKEN", "slack",
		"no Slack token available",
		"export a user token ($%s=xoxp-…) or run `munin login slack`")
}

func SlackAppToken(store TokenStore, envName string) (string, error) {
	return slackTokenFor(store, envName, "SLACK_APP_TOKEN", "slack-app",
		"no Slack app-level token available",
		"export an app-level token ($%s=xapp-…) with connections:write to enable Socket Mode")
}

func SlackBotToken(store TokenStore, envName string) (string, error) {
	return slackTokenFor(store, envName, "SLACK_BOT_TOKEN", "slack-bot",
		"no Slack bot token available",
		"export a bot token ($%s=xoxb-…) to enable Socket Mode")
}

func SlackLogin(ctx context.Context, sa SlackAuth, w io.Writer) error {
	if sa.ClientID == "" || sa.ClientSecret == "" {
		return errs.New(errs.KindConfig, "missing Slack OAuth app client credentials").
			WithHint("set `slack.oauth_client_id` and `slack.oauth_client_secret` in config to use `munin login slack`")
	}
	scopes := sa.UserScopes
	if scopes == "" {
		scopes = DefaultSlackUserScopes
	}

	verifier, err := slackVerifier()
	if err != nil {
		return errs.Wrap(errs.KindAuth, err, "generating pkce verifier")
	}
	challenge := slackChallenge(verifier)

	code, redirect, err := loopbackAuthCode(ctx, w, "Slack", func(redirect, state string) string {
		q := url.Values{
			"client_id":             {sa.ClientID},
			"user_scope":            {scopes},
			"redirect_uri":          {redirect},
			"state":                 {state},
			"code_challenge":        {challenge},
			"code_challenge_method": {"S256"},
		}
		return slackAuthorizeURL + "?" + q.Encode()
	})
	if err != nil {
		return err
	}

	token, err := slackExchange(ctx, http.DefaultClient, sa, code, redirect, verifier)
	if err != nil {
		return err
	}
	return sa.Store.Put("slack", Credential{AccessToken: token, Scope: scopes})
}

func slackVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func slackChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func slackExchange(ctx context.Context, hc *http.Client, sa SlackAuth, code, redirect, verifier string) (string, error) {
	form := url.Values{
		"client_id":     {sa.ClientID},
		"client_secret": {sa.ClientSecret},
		"code":          {code},
		"redirect_uri":  {redirect},
		"code_verifier": {verifier},
	}
	var resp struct {
		OK         bool   `json:"ok"`
		Error      string `json:"error"`
		AuthedUser struct {
			AccessToken string `json:"access_token"`
		} `json:"authed_user"`
	}
	if err := postForm(ctx, hc, slackTokenURL, form, &resp); err != nil {
		return "", errs.Wrap(errs.KindAuth, err, "exchanging code with Slack")
	}
	if !resp.OK {
		return "", errs.Newf(errs.KindAuth, "slack oauth failed: %s", resp.Error)
	}
	if resp.AuthedUser.AccessToken == "" {
		return "", errs.New(errs.KindAuth, "slack returned no user token").
			WithHint("check the Slack app's configured user scopes")
	}
	return resp.AuthedUser.AccessToken, nil
}
