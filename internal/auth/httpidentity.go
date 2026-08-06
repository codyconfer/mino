package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/codyconfer/mino/internal/errs"
)

const (
	minDevicePollInterval = 5 * time.Second
	maxDevicePollInterval = 60 * time.Second
	maxDeviceLifetime     = 15 * time.Minute
)

type DeviceStart struct {
	DeviceCode              string
	UserCode                string
	VerificationURI         string
	VerificationURIComplete string
	Interval                time.Duration
	ExpiresIn               time.Duration
}

type DevicePoll struct {
	AccessToken string
	Scope       string
	Pending     bool
	SlowDown    bool
	Denied      bool
	Expired     bool
}

type GitHubIdentity struct {
	Login string
	ID    int64
	Type  string
}

func GitHubOAuthEndpoints(apiURL string) (deviceURL, tokenURL string, err error) {
	raw := strings.TrimSpace(apiURL)
	if raw == "" {
		return githubDeviceCodeURL, githubAccessTokenURL, nil
	}
	u, perr := url.Parse(raw)
	if perr != nil || u.Host == "" {
		return "", "", errs.Newf(errs.KindConfig, "github.api_url %q is not a URL", apiURL)
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return "", "", errs.Newf(errs.KindConfig, "refusing to run a device flow over %q", u.Scheme).
			WithHint("github.api_url must use https: the device code and the access token both cross it")
	}
	host := GHHostname(raw)
	if host == "" {
		return "", "", errs.Newf(errs.KindConfig, "github.api_url %q has no host", apiURL)
	}
	if host == "github.com" {
		return githubDeviceCodeURL, githubAccessTokenURL, nil
	}
	return "https://" + host + "/login/device/code", "https://" + host + "/login/oauth/access_token", nil
}

func GitHubDeviceStart(ctx context.Context, deviceURL, clientID, scope string) (DeviceStart, error) {
	if strings.TrimSpace(clientID) == "" {
		return DeviceStart{}, errs.New(errs.KindConfig, "no OAuth client id configured for api login").
			WithHint("set daemon.http.identity.client_id to an OAuth App client id with device flow enabled")
	}
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
	}
	status, err := githubPostForm(ctx, deviceURL, form, &resp)
	if err != nil {
		return DeviceStart{}, err
	}
	if resp.Error != "" {
		return DeviceStart{}, deviceFlowError(resp.Error, status)
	}
	if status >= 400 {
		return DeviceStart{}, clientIDError(status)
	}
	if resp.DeviceCode == "" || resp.UserCode == "" {
		return DeviceStart{}, errs.New(errs.KindSignal, "the device authorization response carried no code")
	}
	uri := resp.VerificationURI
	if uri == "" {
		uri = verificationFallback(deviceURL)
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

func GitHubDevicePoll(ctx context.Context, tokenURL, clientID, deviceCode string) (DevicePoll, error) {
	form := url.Values{
		"client_id":   {clientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}
	var resp struct {
		AccessToken string `json:"access_token"`
		Scope       string `json:"scope"`
		Error       string `json:"error"`
	}
	status, err := githubPostForm(ctx, tokenURL, form, &resp)
	if err != nil {
		return DevicePoll{}, err
	}
	if resp.Error == "" && status >= 400 {
		return DevicePoll{}, clientIDError(status)
	}
	switch resp.Error {
	case "":
		if resp.AccessToken == "" {
			return DevicePoll{Pending: true}, nil
		}
		return DevicePoll{AccessToken: resp.AccessToken, Scope: resp.Scope}, nil
	case "authorization_pending":
		return DevicePoll{Pending: true}, nil
	case "slow_down":
		return DevicePoll{Pending: true, SlowDown: true}, nil
	case "access_denied":
		return DevicePoll{Denied: true}, nil
	case "expired_token":
		return DevicePoll{Expired: true}, nil
	default:
		return DevicePoll{}, deviceFlowError(resp.Error, status)
	}
}

func GitHubWhoAmI(ctx context.Context, apiURL, tok string) (GitHubIdentity, error) {
	base := strings.TrimRight(strings.TrimSpace(apiURL), "/")
	if base == "" {
		base = "https://api.github.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/user", nil)
	if err != nil {
		return GitHubIdentity{}, errs.Wrap(errs.KindSignal, err, "github: building the identity request")
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		return GitHubIdentity{}, errs.Wrap(errs.KindSignal, err, "github: resolving the caller identity")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return GitHubIdentity{}, classifyGitHubStatus(resp, errorExcerpt(resp.Body))
	}
	body, err := readBounded(resp, "github user", maxAPIResponseBytes)
	if err != nil {
		return GitHubIdentity{}, err
	}
	var out struct {
		Login string `json:"login"`
		ID    int64  `json:"id"`
		Type  string `json:"type"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return GitHubIdentity{}, errs.Wrap(errs.KindSignal, err, "github: decoding the identity response")
	}
	if out.Login == "" || out.ID == 0 {
		return GitHubIdentity{}, errs.New(errs.KindSignal, "github returned no login for that token")
	}
	return GitHubIdentity{Login: out.Login, ID: out.ID, Type: out.Type}, nil
}

func oauthPostForm(ctx context.Context, label, endpoint string, form url.Values, clientErr func(int) error, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, errs.Wrapf(errs.KindSignal, err, "%s: building request", label)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		return 0, errs.Wrap(errs.KindSignal, err, label)
	}
	defer resp.Body.Close()
	body, err := readBounded(resp, label, maxTokenResponseBytes)
	if err != nil {
		return resp.StatusCode, err
	}
	if len(body) == 0 {
		if resp.StatusCode >= 400 {
			return resp.StatusCode, clientErr(resp.StatusCode)
		}
		return resp.StatusCode, errs.Newf(errs.KindSignal, "%s: the response was empty", label)
	}
	if err := json.Unmarshal(body, out); err != nil {
		if resp.StatusCode >= 400 {
			return resp.StatusCode, clientErr(resp.StatusCode)
		}
		return resp.StatusCode, errs.Newf(errs.KindSignal, "%s: %s returned no usable JSON", label, resp.Status)
	}
	return resp.StatusCode, nil
}

func githubPostForm(ctx context.Context, endpoint string, form url.Values, out any) (int, error) {
	return oauthPostForm(ctx, "github device flow", endpoint, form, clientIDError, out)
}

func clientIDError(status int) error {
	if status == http.StatusTooManyRequests || status >= 500 {
		return errs.Newf(errs.KindSignal, "github device flow: the forge returned %d", status)
	}
	return errs.Newf(errs.KindConfig, "the forge rejected the device flow request (%d)", status).
		WithHint("check daemon.http.identity.client_id is an OAuth App client id on this host " +
			"with \"Enable Device Flow\" ticked")
}

func verificationFallback(deviceURL string) string {
	if u, err := url.Parse(deviceURL); err == nil && u.Host != "" {
		return u.Scheme + "://" + u.Host + "/login/device"
	}
	return "https://github.com/login/device"
}

func deviceFlowError(code string, status int) error {
	switch code {
	case "device_flow_disabled":
		return errs.New(errs.KindConfig, "that OAuth app does not have device flow enabled").
			WithHint("tick \"Enable Device Flow\" on the OAuth App that owns daemon.http.identity.client_id")
	case "unauthorized_client", "invalid_client":
		return errs.Newf(errs.KindConfig, "the forge rejected the client id (%s)", code).
			WithHint("check daemon.http.identity.client_id is an OAuth App client id on this host")
	case "incorrect_client_credentials":
		return errs.New(errs.KindConfig, "the forge rejected the client credentials").
			WithHint("device flow is a public client: set only daemon.http.identity.client_id, never a secret")
	}
	if status >= 400 && status < 500 && status != http.StatusTooManyRequests {
		return clientIDError(status)
	}
	return errs.Newf(errs.KindSignal, "github device flow failed: %s", code)
}

func clampDuration(d, lo, hi time.Duration) time.Duration {
	if d < lo {
		return lo
	}
	if d > hi {
		return hi
	}
	return d
}
