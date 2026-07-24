package auth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"

	sauth "github.com/codyconfer/sisyphus/auth"

	"github.com/codyconfer/munin/internal/errs"
)

func GHAvailable() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

func GitHubToken(store TokenStore) (token, origin string) {
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t, "$GITHUB_TOKEN"
	}
	if t := os.Getenv("GH_TOKEN"); t != "" {
		return t, "$GH_TOKEN"
	}
	if c, ok := getCred(store, "github"); ok {
		return c.AccessToken, "cached OAuth token"
	}
	return "", ""
}

func CacheGitHubToken(store TokenStore, token, scope string) error {
	return store.Put(context.Background(), "github", Credential{AccessToken: token, Scope: scope})
}

func GitHubAuthStatus(ctx context.Context, store TokenStore) (ok bool, detail string) {
	if GHAvailable() {
		if _, err := GH(ctx, "auth", "status"); err == nil {
			return true, "gh CLI is logged in"
		}
	}
	if tok, origin := GitHubToken(store); tok != "" {
		return true, "using " + origin
	}
	return false, ""
}

var (
	githubDeviceCodeURL  = "https://github.com/login/device/code"
	githubAccessTokenURL = "https://github.com/login/oauth/access_token"
)

func GitHubDeviceFlow(ctx context.Context, clientID, scope string, w io.Writer) (string, error) {
	return runGitHubDeviceFlow(ctx, http.DefaultClient, githubDeviceCodeURL, githubAccessTokenURL, clientID, scope, w, time.Sleep)
}

func runGitHubDeviceFlow(ctx context.Context, hc *http.Client, deviceURL, tokenURL, clientID, scope string, w io.Writer, sleep func(time.Duration)) (string, error) {
	if clientID == "" {
		return "", errs.New(errs.KindConfig, "no GitHub OAuth client id configured").
			WithHint("set a GitHub OAuth App client id in config to use `munin login github`")
	}
	tok, _, err := sauth.DeviceToken(ctx, w, sauth.DeviceFlowOptions{
		ClientID:   clientID,
		Scope:      scope,
		CodeURL:    deviceURL,
		TokenURL:   tokenURL,
		Product:    "munin",
		HTTPClient: hc,
		Sleep:      sleep,
		Open:       func(string) error { return nil }, // munin prints the URL; browser optional
	})
	if err != nil {
		switch {
		case errors.Is(err, sauth.ErrDeviceDenied):
			return "", errs.New(errs.KindAuth, "authorization was denied")
		case errors.Is(err, sauth.ErrDeviceExpired):
			return "", errs.New(errs.KindAuth, "device code expired before authorization").
				WithHint("run `munin login github` again")
		default:
			return "", errs.Wrap(errs.KindAuth, err, "github device flow").
				WithHint("run `munin login github` again")
		}
	}
	return tok, nil
}
