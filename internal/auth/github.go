package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

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
	return store.Put("github", Credential{AccessToken: token, Scope: scope})
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
	var dc struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		Interval        int    `json:"interval"`
		ExpiresIn       int    `json:"expires_in"`
	}
	form := url.Values{"client_id": {clientID}}
	if scope != "" {
		form.Set("scope", scope)
	}
	if err := postForm(ctx, hc, deviceURL, form, &dc); err != nil {
		return "", errs.Wrap(errs.KindAuth, err, "requesting device code").
			WithHint("run `munin login github` again")
	}
	fmt.Fprintf(w, "\nTo authorize munin, open %s\nand enter the code: %s\n\nWaiting for authorization…\n",
		dc.VerificationURI, dc.UserCode)

	interval := dc.Interval
	if interval <= 0 {
		interval = 5
	}
	maxPolls := 12
	if dc.ExpiresIn > 0 {
		maxPolls = dc.ExpiresIn/interval + 1
	}
	poll := url.Values{
		"client_id":   {clientID},
		"device_code": {dc.DeviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}
	for i := 0; i < maxPolls; i++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		sleep(time.Duration(interval) * time.Second)

		var tr struct {
			AccessToken      string `json:"access_token"`
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		if err := postForm(ctx, hc, tokenURL, poll, &tr); err != nil {
			return "", errs.Wrap(errs.KindAuth, err, "polling for token")
		}
		switch tr.Error {
		case "":
			if tr.AccessToken != "" {
				return tr.AccessToken, nil
			}
		case "authorization_pending":
		case "slow_down":
			interval += 5
		case "expired_token":
			return "", errs.New(errs.KindAuth, "device code expired before authorization").
				WithHint("run `munin login github` again")
		case "access_denied":
			return "", errs.New(errs.KindAuth, "authorization was denied")
		default:
			return "", errs.Newf(errs.KindAuth, "device flow failed: %s (%s)", tr.Error, tr.ErrorDescription)
		}
	}
	return "", errs.New(errs.KindAuth, "timed out waiting for authorization").
		WithHint("run `munin login github` again")
}

func postForm(ctx context.Context, hc *http.Client, endpoint string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return errs.Wrap(errs.KindAuth, err, "building OAuth request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return errs.Wrap(errs.KindAuth, err, "OAuth request failed")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return errs.Newf(errs.KindAuth, "%s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return json.Unmarshal(body, out)
}
