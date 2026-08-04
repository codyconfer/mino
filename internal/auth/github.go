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

	"github.com/codyconfer/mino/internal/errs"
)

func GHAvailable() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

const (
	originGitHubToken = "$GITHUB_TOKEN"
	originGHToken     = "$GH_TOKEN"
	originStoredToken = "cached OAuth token"
)

func GitHubToken(store TokenStore) (token, origin string) {
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t, originGitHubToken
	}
	if t := os.Getenv("GH_TOKEN"); t != "" {
		return t, originGHToken
	}
	if c, ok := getCred(store, "github"); ok {
		return c.AccessToken, originStoredToken
	}
	return "", ""
}

func CacheGitHubToken(store TokenStore, token, scope string) error {
	return store.Put(context.Background(), "github", Credential{AccessToken: token, Scope: scope})
}

func GitHubAuthStatus(ctx context.Context, sel GitHubSelection) (ok bool, detail string) {
	if sel.UsesGHCLI() {
		args := append([]string{"auth", "status"}, GHHostFlag(sel.APIURL)...)
		if _, err := GH(ctx, args...); err == nil {
			return true, "gh CLI is logged in"
		}
		return false, ""
	}
	if !sel.Authenticated() {
		return false, ""
	}
	if _, err := sel.Token(ctx); err != nil {
		return false, "using " + sel.Origin + ": " + err.Error()
	}
	return true, "using " + sel.Origin
}

var (
	githubDeviceCodeURL  = "https://github.com/login/device/code"
	githubAccessTokenURL = "https://github.com/login/oauth/access_token"
)

func GitHubDeviceFlow(ctx context.Context, clientID, scope string, w io.Writer) (string, error) {
	return runGitHubDeviceFlow(ctx, deviceFlowHTTPClient, githubDeviceCodeURL, githubAccessTokenURL, clientID, scope, w, time.Sleep)
}

func runGitHubDeviceFlow(ctx context.Context, hc *http.Client, deviceURL, tokenURL, clientID, scope string, w io.Writer, sleep func(time.Duration)) (string, error) {
	if clientID == "" {
		return "", errs.New(errs.KindConfig, "no GitHub OAuth client id configured").
			WithHint("set a GitHub OAuth App client id in config to use `mino login github`")
	}
	tok, _, err := sauth.DeviceToken(ctx, w, sauth.DeviceFlowOptions{
		ClientID:   clientID,
		Scope:      scope,
		CodeURL:    deviceURL,
		TokenURL:   tokenURL,
		Product:    "mino",
		HTTPClient: hc,
		Sleep:      sleep,
		Open:       func(string) error { return nil },
	})
	if err != nil {
		switch {
		case errors.Is(err, sauth.ErrDeviceDenied):
			return "", errs.New(errs.KindAuth, "authorization was denied")
		case errors.Is(err, sauth.ErrDeviceExpired):
			return "", errs.New(errs.KindAuth, "device code expired before authorization").
				WithHint("run `mino login github` again")
		default:
			return "", errs.Wrap(errs.KindAuth, err, "github device flow").
				WithHint("run `mino login github` again")
		}
	}
	return tok, nil
}
