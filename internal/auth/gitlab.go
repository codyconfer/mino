package auth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/codyconfer/mino/internal/errs"
)

const (
	originGitLabToken  = "$GITLAB_TOKEN"
	originGLToken      = "$GL_TOKEN"
	originGitLabStored = "cached OAuth token"
)

const (
	gitlabDevicePath     = "/oauth/authorize_device"
	gitlabTokenPath      = "/oauth/token"
	gitlabDeviceUserPath = "/oauth/device"
)

func GitLabToken(store TokenStore) (token, origin string) {
	if t := os.Getenv("GITLAB_TOKEN"); t != "" {
		return t, originGitLabToken
	}
	if t := os.Getenv("GL_TOKEN"); t != "" {
		return t, originGLToken
	}
	if c, ok := getCred(store, gitlabCredKey); ok {
		return c.AccessToken, originGitLabStored
	}
	return "", ""
}

func GitLabCredential(store TokenStore) (Credential, bool) {
	return getCred(store, gitlabCredKey)
}

func CacheGitLabCredential(store TokenStore, c Credential) error {
	if store == nil {
		return nil
	}
	return store.Put(context.Background(), gitlabCredKey, c)
}

func GitLabOAuthEndpoints(apiURL string) (deviceURL, tokenURL string, err error) {
	raw := strings.TrimSpace(apiURL)
	if raw == "" {
		base := "https://" + defaultGitLabHost
		return base + gitlabDevicePath, base + gitlabTokenPath, nil
	}
	u, perr := url.Parse(raw)
	if perr != nil {
		return "", "", errs.Wrapf(errs.KindConfig, perr, "gitlab: invalid api_url %q", raw)
	}
	if u.Scheme != "https" {
		return "", "", errs.Newf(errs.KindConfig, "gitlab: api_url must use https (refusing to send token over %q)", raw).
			WithHint("set gitlab.api_url to an https:// endpoint, e.g. https://gitlab.example.com")
	}
	if u.Host == "" {
		return "", "", errs.Newf(errs.KindConfig, "gitlab: api_url has no host: %q", raw)
	}
	base := GitLabInstanceURL(raw)
	return base + gitlabDevicePath, base + gitlabTokenPath, nil
}

type gitlabTokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	Scope            string `json:"scope"`
	ExpiresIn        int    `json:"expires_in"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func (r gitlabTokenResponse) credential(now time.Time) Credential {
	c := Credential{AccessToken: r.AccessToken, RefreshToken: r.RefreshToken, Scope: r.Scope}
	if r.ExpiresIn > 0 {
		c.Expiry = now.Add(time.Duration(r.ExpiresIn) * time.Second)
	}
	return c
}

func GitLabDeviceFlow(ctx context.Context, apiURL, clientID, scope string, w io.Writer) (Credential, error) {
	deviceURL, tokenURL, err := GitLabOAuthEndpoints(apiURL)
	if err != nil {
		return Credential{}, err
	}
	return runGitLabDeviceFlow(ctx, deviceURL, tokenURL, clientID, scope, w, time.Sleep)
}

func runGitLabDeviceFlow(ctx context.Context, deviceURL, tokenURL, clientID, scope string, w io.Writer, sleep func(time.Duration)) (Credential, error) {
	if strings.TrimSpace(clientID) == "" {
		return Credential{}, errs.New(errs.KindConfig, "gitlab.oauth_client_id is not set").
			WithHint("set `gitlab.oauth_client_id` to a GitLab OAuth application id; the application " +
				"must not be marked Confidential, because device flow has nowhere to put a secret")
	}

	start, err := gitlabDeviceStart(ctx, deviceURL, clientID, scope)
	if err != nil {
		return Credential{}, err
	}
	promptGitLabDevice(w, start)

	interval := start.Interval
	deadline := time.Now().Add(start.ExpiresIn)
	for {
		if err := ctx.Err(); err != nil {
			return Credential{}, err
		}
		if time.Now().After(deadline) {
			return Credential{}, errs.New(errs.KindAuth, "device code expired before authorization").
				WithHint("run `mino login gitlab` again")
		}
		sleep(interval)

		cred, pending, slower, err := gitlabDevicePoll(ctx, tokenURL, clientID, start.DeviceCode)
		switch {
		case err != nil:
			return Credential{}, err
		case slower:
			interval = clampDuration(interval+minDevicePollInterval, minDevicePollInterval, maxDevicePollInterval)
		case pending:
		default:
			return cred, nil
		}
	}
}

func gitlabDeviceStart(ctx context.Context, deviceURL, clientID, scope string) (DeviceStart, error) {
	form := url.Values{"client_id": {clientID}}
	if s := strings.TrimSpace(scope); s != "" {
		form.Set("scope", s)
	}
	var resp struct {
		DeviceCode              string `json:"device_code"`
		UserCode                string `json:"user_code"`
		VerificationURI         string `json:"verification_uri"`
		VerificationURIComplete string `json:"verification_uri_complete"`
		ExpiresIn               int    `json:"expires_in"`
		Interval                int    `json:"interval"`
		Error                   string `json:"error"`
		ErrorDescription        string `json:"error_description"`
	}
	status, err := oauthPostForm(ctx, "gitlab device flow", deviceURL, form, gitlabClientError, &resp)
	if err != nil {
		return DeviceStart{}, err
	}
	if resp.Error != "" {
		return DeviceStart{}, gitlabOAuthError(resp.Error, resp.ErrorDescription, status)
	}
	if status >= 400 {
		return DeviceStart{}, gitlabClientError(status)
	}
	if resp.DeviceCode == "" || resp.UserCode == "" {
		return DeviceStart{}, errs.New(errs.KindSignal, "the device authorization response carried no code")
	}
	uri := resp.VerificationURI
	if uri == "" {
		uri = GitLabInstanceURL(deviceURL) + gitlabDeviceUserPath
	}
	return DeviceStart{
		DeviceCode:              resp.DeviceCode,
		UserCode:                resp.UserCode,
		VerificationURI:         uri,
		VerificationURIComplete: resp.VerificationURIComplete,
		Interval:                clampDuration(time.Duration(resp.Interval)*time.Second, minDevicePollInterval, maxDevicePollInterval),
		ExpiresIn:               clampDuration(time.Duration(resp.ExpiresIn)*time.Second, minDevicePollInterval, maxDeviceLifetime),
	}, nil
}

func gitlabDevicePoll(ctx context.Context, tokenURL, clientID, deviceCode string) (cred Credential, pending, slower bool, err error) {
	var resp gitlabTokenResponse
	status, err := oauthPostForm(ctx, "gitlab device flow", tokenURL, url.Values{
		"client_id":   {clientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}, gitlabClientError, &resp)
	if err != nil {
		return Credential{}, false, false, err
	}
	switch resp.Error {
	case "":
		if resp.AccessToken == "" {
			if status >= 400 {
				return Credential{}, false, false, gitlabClientError(status)
			}
			return Credential{}, true, false, nil
		}
		return resp.credential(time.Now()), false, false, nil
	case "authorization_pending":
		return Credential{}, true, false, nil
	case "slow_down":
		return Credential{}, true, true, nil
	}
	return Credential{}, false, false, gitlabOAuthError(resp.Error, resp.ErrorDescription, status)
}

func promptGitLabDevice(w io.Writer, start DeviceStart) {
	if w == nil {
		return
	}
	uri := start.VerificationURIComplete
	if uri == "" {
		uri = start.VerificationURI
	}
	fmt.Fprintf(w, "Open %s and enter code %s\n", uri, start.UserCode)
}

func refreshGitLabToken(ctx context.Context, instanceURL, clientID, refreshToken string) (Credential, error) {
	var resp gitlabTokenResponse
	status, err := oauthPostForm(ctx, "gitlab token refresh", instanceURL+gitlabTokenPath, url.Values{
		"client_id":     {clientID},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
	}, gitlabClientError, &resp)
	if err != nil {
		return Credential{}, err
	}
	if resp.Error != "" {
		return Credential{}, gitlabOAuthError(resp.Error, resp.ErrorDescription, status)
	}
	if resp.AccessToken == "" {
		return Credential{}, errs.New(errs.KindAuth, "gitlab returned no access token when refreshing").
			WithHint("run `mino login gitlab` again")
	}
	return resp.credential(time.Now()), nil
}

func gitlabClientError(status int) error {
	if status == http.StatusTooManyRequests || status >= 500 {
		return errs.Newf(errs.KindSignal, "gitlab oauth: the instance returned %d", status)
	}
	return errs.Newf(errs.KindConfig, "gitlab rejected the oauth request (%d)", status).
		WithHint("check gitlab.oauth_client_id is an application id on this instance and that the " +
			"application is not marked Confidential")
}

func gitlabOAuthError(code, desc string, status int) error {
	detail := code
	if desc != "" {
		detail = code + ": " + desc
	}
	switch code {
	case "unauthorized_client", "invalid_client":
		return errs.Newf(errs.KindConfig, "gitlab rejected the client id (%s)", detail).
			WithHint("untick \"Confidential\" on the OAuth application that owns gitlab.oauth_client_id, " +
				"and make sure it allows the device authorization grant")
	case "access_denied":
		return errs.New(errs.KindAuth, "authorization was denied")
	case "expired_token":
		return errs.New(errs.KindAuth, "device code expired before authorization").
			WithHint("run `mino login gitlab` again")
	case "invalid_grant":
		return errs.Newf(errs.KindAuth, "gitlab rejected the grant (%s)", detail).
			WithHint("the cached refresh token is no longer valid; run `mino login gitlab` again")
	case "invalid_scope":
		return errs.Newf(errs.KindConfig, "gitlab rejected the requested scopes (%s)", detail).
			WithHint("set gitlab.oauth_scopes to scopes the application grants, e.g. \"read_api read_user\"")
	}
	if status >= 400 && status < 500 && status != http.StatusTooManyRequests {
		return gitlabClientError(status)
	}
	return errs.Newf(errs.KindAuth, "gitlab oauth failed: %s", detail)
}
