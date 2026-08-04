package auth

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
)

const (
	appJWTHeader     = "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9"
	appJWTLifetime   = 9 * time.Minute
	appJWTBackdate   = 60 * time.Second
	appMinKeyBits    = 2048
	appRefreshMargin = 5 * time.Minute
	appExchangeGrace = 10 * time.Second
)

func decodeAppKey(raw string) []byte {
	if strings.Contains(raw, "-----BEGIN") {
		return []byte(raw)
	}
	if b, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(raw), "")); err == nil {
		return b
	}
	return []byte(raw)
}

func parseAppKey(data []byte, source string) (*rsa.PrivateKey, error) {
	bad := func(format string, args ...any) *errs.Error {
		return errs.Newf(errs.KindConfig, "github app: "+format, args...)
	}
	block, _ := pem.Decode(bytes.TrimSpace(data))
	if block == nil {
		return nil, bad("the key in %s is not PEM", source).
			WithHint("download the .pem from Settings > Developer settings > GitHub Apps > your app > Private keys")
	}
	if block.Type == "OPENSSH PRIVATE KEY" {
		return nil, bad("%s holds an SSH private key, not a GitHub App key", source).
			WithHint("this is your commit-signing key; the App key is a separate .pem from the App's settings page")
	}
	if _, encrypted := block.Headers["Proc-Type"]; encrypted {
		return nil, bad("the key in %s is passphrase-protected", source).
			WithHint("GitHub App keys are never encrypted, so this is a different key")
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		parsed, err8 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err8 != nil {
			return nil, bad("the key in %s could not be parsed as PKCS#1 or PKCS#8: %v", source, err)
		}
		rsaKey, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, bad("the key in %s is %T, not RSA", source, parsed).
				WithHint("GitHub App keys are RSA")
		}
		key = rsaKey
	}
	if key.N.BitLen() < appMinKeyBits {
		return nil, bad("the key in %s is only %d bits", source, key.N.BitLen()).
			WithHint("GitHub App keys are at least %d bits, so this file is truncated or not an App key", appMinKeyBits)
	}
	return key, nil
}

func appJWT(key *rsa.PrivateKey, issuer string, now time.Time) (string, error) {
	var iss any = issuer
	if n, err := strconv.Atoi(strings.TrimSpace(issuer)); err == nil {
		iss = n
	}
	claims, err := json.Marshal(map[string]any{
		"iat": now.Add(-appJWTBackdate).Unix(),
		"exp": now.Add(appJWTLifetime).Unix(),
		"iss": iss,
	})
	if err != nil {
		return "", errs.Wrap(errs.KindInternal, err, "github app: encoding jwt claims")
	}
	signing := appJWTHeader + "." + base64.RawURLEncoding.EncodeToString(claims)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(nil, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", errs.Wrap(errs.KindAuth, err, "github app: signing jwt")
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func appBase(apiURL string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(apiURL), "/")
	if base == "" {
		return "https://api.github.com", nil
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return "", errs.Newf(errs.KindConfig, "github app: github.api_url %q is not a URL", apiURL)
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return "", errs.Newf(errs.KindConfig, "github app: refusing to send a signed JWT over %q", u.Scheme).
			WithHint("github.api_url must use https")
	}
	return base, nil
}

type appSource struct {
	spec GitHubSpec
	base string
	http *http.Client
	now  func() time.Time

	mu      sync.Mutex
	key     *rsa.PrivateKey
	keyFrom string
	install string
	token   string
	expiry  time.Time
	pending *appExchange
}

type appExchange struct {
	done   chan struct{}
	token  string
	expiry time.Time
	err    error
}

func newAppSource(spec GitHubSpec) (GitHubSource, string, error) {
	app := spec.App
	if strings.TrimSpace(app.ID) == "" {
		return nil, "", errs.New(errs.KindConfig, "github.app is configured but github.app.id is not set").
			WithHint(appPartialHint)
	}
	if _, err := strconv.Atoi(strings.TrimSpace(app.ID)); err != nil {
		return nil, "", errs.Newf(errs.KindConfig, "github.app.id %q is not numeric", app.ID).
			WithHint("use the numeric App ID from the App's settings page, not its slug")
	}
	if id := strings.TrimSpace(app.InstallationID); id != "" {
		if _, err := strconv.Atoi(id); err != nil {
			return nil, "", errs.Newf(errs.KindConfig, "github.app.installation_id %q is not numeric", id).
				WithHint("use the numeric installation id, not the account name")
		}
	}
	pemBytes, keyFrom, err := appKeyPEM(spec)
	if err != nil {
		return nil, "", err
	}
	if len(pemBytes) == 0 {
		return nil, "", errs.New(errs.KindConfig, "github.app is configured but no private key is reachable").
			WithHint(appPartialHint)
	}
	key, err := parseAppKey(pemBytes, keyFrom)
	if err != nil {
		return nil, "", err
	}
	base, err := appBase(spec.APIURL)
	if err != nil {
		return nil, "", err
	}

	src := &appSource{
		spec:    spec,
		base:    base,
		http:    HTTPClient(),
		now:     time.Now,
		key:     key,
		keyFrom: keyFrom,
		install: strings.TrimSpace(app.InstallationID),
	}
	origin := "GitHub App " + strings.TrimSpace(app.ID)
	if src.install != "" {
		origin += " installation " + src.install
	}
	return src, origin, nil
}

const appPartialHint = "set github.app.id and github.app.private_key_path (or MINO_GITHUB_APP_ID and " +
	"MINO_GITHUB_APP_PRIVATE_KEY_PATH); mino will not fall back to your personal GitHub credential " +
	"when a GitHub App is configured"

func (p *appSource) Invalidate() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.token, p.expiry = "", time.Time{}
}

func (p *appSource) Token(ctx context.Context) (string, error) {
	p.mu.Lock()
	if p.token != "" && p.now().Before(p.expiry.Add(-appRefreshMargin)) {
		tok := p.token
		p.mu.Unlock()
		return tok, nil
	}
	ex := p.pending
	if ex == nil {
		ex = &appExchange{done: make(chan struct{})}
		p.pending = ex
		go p.exchange(ctx, ex)
	}
	p.mu.Unlock()

	select {
	case <-ex.done:
		if ex.err != nil {
			return "", ex.err
		}
		return ex.token, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (p *appSource) exchange(ctx context.Context, ex *appExchange) {
	ectx, cancel := context.WithTimeout(context.WithoutCancel(ctx), appExchangeGrace)
	defer cancel()

	tok, exp, err := p.mint(ectx)

	p.mu.Lock()
	ex.token, ex.expiry, ex.err = tok, exp, err
	if err == nil {
		p.token, p.expiry = tok, exp
	}
	p.pending = nil
	p.mu.Unlock()
	close(ex.done)
}

func (p *appSource) mint(ctx context.Context) (string, time.Time, error) {
	jwt, err := appJWT(p.key, strings.TrimSpace(p.spec.App.ID), p.now())
	if err != nil {
		return "", time.Time{}, err
	}
	install, err := p.installation(ctx, jwt)
	if err != nil {
		return "", time.Time{}, err
	}

	body, err := p.appAPI(ctx, http.MethodPost, "/app/installations/"+install+"/access_tokens", jwt)
	if err != nil {
		return "", time.Time{}, err
	}
	var out struct {
		Token       string            `json:"token"`
		ExpiresAt   time.Time         `json:"expires_at"`
		Permissions map[string]string `json:"permissions"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", time.Time{}, errs.Wrap(errs.KindSignal, err, "github app: decoding the installation token")
	}
	if out.Token == "" {
		return "", time.Time{}, errs.New(errs.KindAuth, "github app: the installation token response was empty")
	}
	perms := make([]string, 0, len(out.Permissions))
	for k := range out.Permissions {
		perms = append(perms, k)
	}
	slices.Sort(perms)
	log.Debugf("github app: installation %s token expires %s, permissions: %s",
		install, out.ExpiresAt.Format(time.RFC3339), strings.Join(perms, " "))
	return out.Token, out.ExpiresAt, nil
}

func (p *appSource) installation(ctx context.Context, jwt string) (string, error) {
	p.mu.Lock()
	known := p.install
	p.mu.Unlock()
	if known != "" {
		return known, nil
	}

	body, err := p.appAPI(ctx, http.MethodGet, "/app/installations", jwt)
	if err != nil {
		return "", err
	}
	var found []struct {
		ID      int64 `json:"id"`
		Account struct {
			Login string `json:"login"`
		} `json:"account"`
	}
	if err := json.Unmarshal(body, &found); err != nil {
		return "", errs.Wrap(errs.KindSignal, err, "github app: decoding the installation list")
	}
	switch len(found) {
	case 0:
		return "", errs.New(errs.KindConfig, "github app: the app is not installed on any account").
			WithHint("install it from the app's public page, then set github.app.installation_id")
	case 1:
		id := strconv.FormatInt(found[0].ID, 10)
		log.Infof("github app: using installation %s (%s); pin it with github.app.installation_id",
			id, found[0].Account.Login)
		p.mu.Lock()
		p.install = id
		p.mu.Unlock()
		return id, nil
	}
	names := make([]string, 0, len(found))
	for _, f := range found {
		names = append(names, fmt.Sprintf("%d (%s)", f.ID, f.Account.Login))
	}
	return "", errs.Newf(errs.KindConfig, "github app: the app has %d installations: %s",
		len(found), strings.Join(names, ", ")).
		WithHint("set github.app.installation_id to the one mino should act as")
}

func (p *appSource) appAPI(ctx context.Context, method, path, jwt string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, p.base+path, nil)
	if err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "github app: building request")
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := p.http.Do(req)
	if err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "github app: request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, p.appStatusError(resp)
	}
	return readBounded(resp, "github app", maxAPIResponseBytes)
}

func (p *appSource) appStatusError(resp *http.Response) error {
	excerpt := errorExcerpt(resp.Body)
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return errs.Newf(errs.KindAuth, "github app: %s rejected the app jwt: %s", p.base, excerpt).
			WithHint("check the system clock, and that github.app.id matches the key in %s", p.keyFrom)
	case http.StatusNotFound:
		return errs.Newf(errs.KindConfig, "github app: no such installation: %s", excerpt).
			WithHint("the app may not be installed on that account, or github.app.installation_id is wrong")
	}
	return classifyGitHubStatus(resp, excerpt)
}
