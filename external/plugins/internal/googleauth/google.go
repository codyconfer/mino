package googleauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"

	"github.com/codyconfer/munin/external/plugins/internal/errx"
	"github.com/codyconfer/munin/external/plugins/internal/httpx"
	"github.com/codyconfer/munin/plugin"
)

var LoginScopes = []string{
	"https://www.googleapis.com/auth/calendar.readonly",
	"https://www.googleapis.com/auth/gmail.readonly",
	"https://www.googleapis.com/auth/drive.metadata.readonly",
	"https://www.googleapis.com/auth/drive.file",
	"https://www.googleapis.com/auth/drive.appdata",
	"https://www.googleapis.com/auth/tasks",
	"openid", "email",
}

var endpoint = google.Endpoint

type Auth struct {
	Store        plugin.CredentialStore
	ClientID     string
	ClientSecret string
}

var scopesNotRequiringGrant = map[string]bool{
	"openid": true,
	"email":  true,
	"https://www.googleapis.com/auth/userinfo.email": true,
}

func ClientOption(ctx context.Context, ga Auth, scopes ...string) (option.ClientOption, error) {
	opt, adcErr := adcOption(ctx, scopes)
	if adcErr == nil {
		return opt, nil
	}
	if tok := readToken(ga.Store); tok != nil {
		return option.WithTokenSource(tokenSource(ctx, ga, scopes, tok)), nil
	}
	return nil, adcErr
}

func Service[T any](ctx context.Context, ga Auth, name string, scopes []string, newSvc func(context.Context, ...option.ClientOption) (*T, error)) (*T, error) {
	opt, err := ClientOption(ctx, ga, scopes...)
	if err != nil {
		return nil, err
	}
	svc, err := newSvc(ctx, opt)
	if err != nil {
		return nil, errx.Wrap(err, name+": creating service")
	}
	return svc, nil
}

func adcOption(ctx context.Context, scopes []string) (option.ClientOption, error) {
	creds, err := google.FindDefaultCredentials(ctx, scopes...)
	if err != nil {
		return nil, adcHelp(scopes, fmt.Sprintf("no Application Default Credentials found (%v)", err))
	}
	tok, err := creds.TokenSource.Token()
	if err != nil {
		return nil, adcHelp(scopes, fmt.Sprintf("could not obtain a token from ADC (%v)", err))
	}
	if missing := missingScopes(ctx, tok.AccessToken, scopes); len(missing) > 0 {
		return nil, adcHelp(scopes, fmt.Sprintf("your credentials are missing required scopes: %s", strings.Join(missing, ", ")))
	}
	return option.WithTokenSource(creds.TokenSource), nil
}

func oauthCtx(ctx context.Context) context.Context {
	return context.WithValue(ctx, oauth2.HTTPClient, httpx.Client())
}

func tokenSource(ctx context.Context, ga Auth, scopes []string, tok *oauth2.Token) oauth2.TokenSource {
	if ga.ClientID != "" && ga.ClientSecret != "" {
		src := &persistingTokenSource{
			store: ga.Store,
			src:   conf(ga, scopes).TokenSource(oauthCtx(ctx), tok),
			last:  tok.AccessToken,
		}
		return oauth2.ReuseTokenSource(tok, src)
	}
	return oauth2.StaticTokenSource(tok)
}

type persistingTokenSource struct {
	store plugin.CredentialStore
	src   oauth2.TokenSource
	last  string
}

func (p *persistingTokenSource) Token() (*oauth2.Token, error) {
	tok, err := p.src.Token()
	if err != nil {
		return nil, err
	}
	if tok.AccessToken != p.last {
		p.last = tok.AccessToken
		if p.store != nil {
			_ = cacheToken(p.store, tok)
		}
	}
	return tok, nil
}

func conf(ga Auth, scopes []string) *oauth2.Config {
	if len(scopes) == 0 {
		scopes = LoginScopes
	}
	return &oauth2.Config{
		ClientID:     ga.ClientID,
		ClientSecret: ga.ClientSecret,
		Endpoint:     endpoint,
		Scopes:       scopes,
	}
}

func Login(ctx context.Context, ga Auth, w io.Writer) error {
	if ga.ClientID == "" || ga.ClientSecret == "" {
		return errx.New("missing Google OAuth desktop-app client credentials").
			WithHint("set `plugins.google.oauth_client_id` and `plugins.google.oauth_client_secret` in config to use `munin login google`")
	}
	conf := conf(ga, LoginScopes)
	verifier := oauth2.GenerateVerifier()
	code, redirect, err := httpx.LoopbackAuthCode(ctx, w, "Google", func(redirect, state string) string {
		conf.RedirectURL = redirect
		return conf.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce, oauth2.S256ChallengeOption(verifier))
	})
	if err != nil {
		return err
	}
	conf.RedirectURL = redirect
	tok, err := conf.Exchange(oauthCtx(ctx), code, oauth2.VerifierOption(verifier))
	if err != nil {
		return errx.Wrap(err, "exchanging authorization code").
			WithHint("run `munin login google` again")
	}
	return cacheToken(ga.Store, tok)
}

func cacheToken(store plugin.CredentialStore, tok *oauth2.Token) error {
	return store.Put(context.Background(), "google", plugin.Credential{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		Scope:        strings.Join(LoginScopes, " "),
		Expiry:       tok.Expiry,
	})
}

func Authed(store plugin.CredentialStore) bool {
	return readToken(store) != nil
}

func readToken(store plugin.CredentialStore) *oauth2.Token {
	c, ok := readCredential(store, "google")
	if !ok {
		return nil
	}
	return &oauth2.Token{AccessToken: c.AccessToken, RefreshToken: c.RefreshToken, Expiry: c.Expiry}
}

var grantedScopes struct {
	mu sync.Mutex
	m  map[string]map[string]bool
}

func tokenScopes(ctx context.Context, accessToken string) map[string]bool {
	grantedScopes.mu.Lock()
	if granted, ok := grantedScopes.m[accessToken]; ok {
		grantedScopes.mu.Unlock()
		return granted
	}
	grantedScopes.mu.Unlock()

	granted := fetchTokenScopes(ctx, accessToken)
	if granted == nil {
		return nil
	}

	grantedScopes.mu.Lock()
	defer grantedScopes.mu.Unlock()
	if grantedScopes.m == nil || len(grantedScopes.m) > 16 {
		grantedScopes.m = map[string]map[string]bool{}
	}
	grantedScopes.m[accessToken] = granted
	return granted
}

func missingScopes(ctx context.Context, accessToken string, required []string) []string {
	granted := tokenScopes(ctx, accessToken)
	if granted == nil {
		return nil
	}
	var missing []string
	for _, s := range required {
		if scopesNotRequiringGrant[s] || granted[s] {
			continue
		}
		missing = append(missing, s)
	}
	return missing
}

var tokenInfoURL = "https://oauth2.googleapis.com/tokeninfo"

func fetchTokenScopes(ctx context.Context, accessToken string) map[string]bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenInfoURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := httpx.Client().Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	body, err := httpx.ReadBounded(resp, "google tokeninfo", httpx.MaxTokenResponseBytes)
	if err != nil {
		return nil
	}
	var info struct {
		Scope string `json:"scope"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return nil
	}
	granted := map[string]bool{}
	for _, s := range strings.Fields(info.Scope) {
		granted[s] = true
	}
	return granted
}

func adcHelp(scopes []string, reason string) error {
	return errx.New(reason).WithHint(
		"authorize Google access with either:\n"+
			"  gcloud auth application-default login \\\n    --scopes=%s\nor:\n  munin login google",
		strings.Join(scopes, ","))
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

func FromHost(h plugin.Host) Auth {
	if h == nil {
		return Auth{}
	}
	s := h.Settings("google")
	return Auth{
		Store:        h.Credentials(),
		ClientID:     plugin.Setting(s, "oauth_client_id", ""),
		ClientSecret: plugin.Setting(s, "oauth_client_secret", ""),
	}
}

func FromBuildContext(bc plugin.BuildContext) (Auth, error) {
	h, ok := plugin.HostOf(bc)
	if !ok {
		return Auth{}, errx.New("google signals require a munin host build context")
	}
	return FromHost(h), nil
}
