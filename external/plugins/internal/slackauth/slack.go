package slackauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/codyconfer/munin/external/plugins/internal/errx"
	"github.com/codyconfer/munin/external/plugins/internal/httpx"
	"github.com/codyconfer/munin/plugin"
)

const DefaultUserScopes = "channels:history,channels:read,groups:history,groups:read"

var (
	authorizeURL = "https://slack.com/oauth/v2/authorize"
	tokenURL     = "https://slack.com/api/oauth.v2.access"
)

type Auth struct {
	Store        plugin.CredentialStore
	ClientID     string
	ClientSecret string
	UserScopes   string
}

func tokenFor(store plugin.CredentialStore, envName, defaultEnv, credKey, notAvailMsg, hint string) (string, error) {
	if envName == "" {
		envName = defaultEnv
	}
	if tok := os.Getenv(envName); tok != "" {
		return tok, nil
	}
	if c, ok := readCredential(store, credKey); ok {
		return c.AccessToken, nil
	}
	return "", errx.New(notAvailMsg).WithHint(hint, envName)
}

func UserToken(store plugin.CredentialStore, envName string) (string, error) {
	return tokenFor(store, envName, "SLACK_TOKEN", "slack",
		"no Slack token available",
		"export a user token ($%s=xoxp-…) or run `munin login slack`")
}

func AppToken(store plugin.CredentialStore, envName string) (string, error) {
	return tokenFor(store, envName, "SLACK_APP_TOKEN", "slack-app",
		"no Slack app-level token available",
		"export an app-level token ($%s=xapp-…) with connections:write to enable Socket Mode")
}

func BotToken(store plugin.CredentialStore, envName string) (string, error) {
	return tokenFor(store, envName, "SLACK_BOT_TOKEN", "slack-bot",
		"no Slack bot token available",
		"export a bot token ($%s=xoxb-…) to enable Socket Mode")
}

func Login(ctx context.Context, sa Auth, w io.Writer) error {
	if sa.ClientID == "" || sa.ClientSecret == "" {
		return errx.New("missing Slack OAuth app client credentials").
			WithHint("set `plugins.slack.oauth_client_id` and `plugins.slack.oauth_client_secret` in config to use `munin login slack`")
	}
	scopes := sa.UserScopes
	if scopes == "" {
		scopes = DefaultUserScopes
	}

	verifier, err := verifier()
	if err != nil {
		return errx.Wrap(err, "generating pkce verifier")
	}
	challenge := challenge(verifier)

	code, redirect, err := httpx.LoopbackAuthCode(ctx, w, "Slack", func(redirect, state string) string {
		q := url.Values{
			"client_id":             {sa.ClientID},
			"user_scope":            {scopes},
			"redirect_uri":          {redirect},
			"state":                 {state},
			"code_challenge":        {challenge},
			"code_challenge_method": {"S256"},
		}
		return authorizeURL + "?" + q.Encode()
	})
	if err != nil {
		return err
	}

	token, err := exchange(ctx, httpx.Client(), sa, code, redirect, verifier)
	if err != nil {
		return err
	}
	return sa.Store.Put(context.Background(), "slack", plugin.Credential{AccessToken: token, Scope: scopes})
}

func verifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func exchange(ctx context.Context, hc *http.Client, sa Auth, code, redirect, verifier string) (string, error) {
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
	if err := httpx.PostForm(ctx, hc, tokenURL, form, &resp); err != nil {
		return "", errx.Wrap(err, "exchanging code with Slack")
	}
	if !resp.OK {
		return "", errx.Newf("slack oauth failed: %s", resp.Error)
	}
	if resp.AuthedUser.AccessToken == "" {
		return "", errx.New("slack returned no user token").
			WithHint("check the Slack app's configured user scopes")
	}
	return resp.AuthedUser.AccessToken, nil
}

func readCredential(store plugin.CredentialStore, service string) (plugin.Credential, bool) {
	if store == nil {
		return plugin.Credential{}, false
	}
	c, ok, err := store.Get(context.Background(), service)
	if err != nil || !ok {
		return plugin.Credential{}, false
	}
	return c, true
}

func Authed(store plugin.CredentialStore, envName string) bool {
	_, err := UserToken(store, envName)
	return err == nil
}

type Config struct {
	Store             plugin.CredentialStore
	TokenEnv          string
	AppTokenEnv       string
	BotTokenEnv       string
	OAuthClientID     string
	OAuthClientSecret string
	UserScopes        string
	Limit             int
}

func FromHost(h plugin.Host) Config {
	if h == nil {
		return Config{}
	}
	s := h.Settings("slack")
	return Config{
		Store:             h.Credentials(),
		TokenEnv:          plugin.Setting(s, "token_env", "SLACK_TOKEN"),
		AppTokenEnv:       plugin.Setting(s, "app_token_env", "SLACK_APP_TOKEN"),
		BotTokenEnv:       plugin.Setting(s, "bot_token_env", "SLACK_BOT_TOKEN"),
		OAuthClientID:     plugin.Setting(s, "oauth_client_id", ""),
		OAuthClientSecret: plugin.Setting(s, "oauth_client_secret", ""),
		UserScopes:        plugin.Setting(s, "user_scopes", ""),
		Limit:             plugin.SettingInt(s, "limit", 50),
	}
}

func FromBuildContext(bc plugin.BuildContext) (Config, error) {
	h, ok := plugin.HostOf(bc)
	if !ok {
		return Config{}, errx.New("slack signals require a munin host build context")
	}
	return FromHost(h), nil
}
