package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"

	"github.com/codyconfer/munin/internal/errs"
)

var GoogleLoginScopes = []string{
	"https://www.googleapis.com/auth/calendar.readonly",
	"https://www.googleapis.com/auth/gmail.readonly",
	"https://www.googleapis.com/auth/drive.metadata.readonly",
	"https://www.googleapis.com/auth/drive",
	"https://www.googleapis.com/auth/drive.appdata",
	"https://www.googleapis.com/auth/tasks",
	"openid", "email",
}

var googleEndpoint = google.Endpoint

type GoogleAuth struct {
	Store        TokenStore
	ClientID     string
	ClientSecret string
}

var scopesNotRequiringGrant = map[string]bool{
	"openid": true,
	"email":  true,
	"https://www.googleapis.com/auth/userinfo.email": true,
}

func GoogleClientOption(ctx context.Context, ga GoogleAuth, scopes ...string) (option.ClientOption, error) {
	opt, adcErr := adcOption(ctx, scopes)
	if adcErr == nil {
		return opt, nil
	}
	if tok := readGoogleToken(ga.Store); tok != nil {
		return option.WithTokenSource(googleTokenSource(ctx, ga, scopes, tok)), nil
	}
	return nil, adcErr
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

func googleTokenSource(ctx context.Context, ga GoogleAuth, scopes []string, tok *oauth2.Token) oauth2.TokenSource {
	if ga.ClientID != "" && ga.ClientSecret != "" {
		src := &persistingGoogleTokenSource{
			store: ga.Store,
			src:   googleConf(ga, scopes).TokenSource(ctx, tok),
			last:  tok.AccessToken,
		}
		return oauth2.ReuseTokenSource(tok, src)
	}
	return oauth2.StaticTokenSource(tok)
}

type persistingGoogleTokenSource struct {
	store TokenStore
	src   oauth2.TokenSource
	last  string
}

func (p *persistingGoogleTokenSource) Token() (*oauth2.Token, error) {
	tok, err := p.src.Token()
	if err != nil {
		return nil, err
	}
	if tok.AccessToken != p.last {
		p.last = tok.AccessToken
		if p.store != nil {
			_ = cacheGoogleToken(p.store, tok)
		}
	}
	return tok, nil
}

func googleConf(ga GoogleAuth, scopes []string) *oauth2.Config {
	if len(scopes) == 0 {
		scopes = GoogleLoginScopes
	}
	return &oauth2.Config{
		ClientID:     ga.ClientID,
		ClientSecret: ga.ClientSecret,
		Endpoint:     googleEndpoint,
		Scopes:       scopes,
	}
}

func GoogleLogin(ctx context.Context, ga GoogleAuth, w io.Writer) error {
	if ga.ClientID == "" || ga.ClientSecret == "" {
		return errs.New(errs.KindConfig, "missing Google OAuth desktop-app client credentials").
			WithHint("set `google.oauth_client_id` and `google.oauth_client_secret` in config to use `munin login google`")
	}
	conf := googleConf(ga, GoogleLoginScopes)
	verifier := oauth2.GenerateVerifier()
	code, redirect, err := loopbackAuthCode(ctx, w, "Google", func(redirect, state string) string {
		conf.RedirectURL = redirect
		return conf.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce, oauth2.S256ChallengeOption(verifier))
	})
	if err != nil {
		return err
	}
	conf.RedirectURL = redirect
	tok, err := conf.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return errs.Wrap(errs.KindAuth, err, "exchanging authorization code").
			WithHint("run `munin login google` again")
	}
	return cacheGoogleToken(ga.Store, tok)
}

func cacheGoogleToken(store TokenStore, tok *oauth2.Token) error {
	return store.Put("google", Credential{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		Scope:        strings.Join(GoogleLoginScopes, " "),
		Expiry:       tok.Expiry,
	})
}

func readGoogleToken(store TokenStore) *oauth2.Token {
	c, ok := getCred(store, "google")
	if !ok {
		return nil
	}
	return &oauth2.Token{AccessToken: c.AccessToken, RefreshToken: c.RefreshToken, Expiry: c.Expiry}
}

func missingScopes(ctx context.Context, accessToken string, required []string) []string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://oauth2.googleapis.com/tokeninfo", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var info struct {
		Scope string `json:"scope"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil
	}
	granted := map[string]bool{}
	for _, s := range strings.Fields(info.Scope) {
		granted[s] = true
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

func adcHelp(scopes []string, reason string) error {
	return errs.New(errs.KindAuth, reason).WithHint(
		"authorize Google access with either:\n"+
			"  gcloud auth application-default login \\\n    --scopes=%s\nor:\n  munin login google",
		strings.Join(scopes, ","))
}
