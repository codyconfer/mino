package loginflow

import (
	"context"
	"io"
	"slices"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/auth"
	"github.com/codyconfer/munin/internal/config"
)

type CredField struct {
	Key    string
	Label  string
	Cur    func(*app.App) string
	Secret bool
}

type Provider struct {
	Key     string
	Label   string
	Signals []string
	Fields  []CredField
	Authed  func(*app.App) bool
	Login   func(ctx context.Context, a *app.App, creds map[string]string, w io.Writer) error
}

func (p Provider) Missing(a *app.App) []CredField {
	var out []CredField
	for _, f := range p.Fields {
		if f.Cur(a) == "" {
			out = append(out, f)
		}
	}
	return out
}

func Providers() []Provider {
	return []Provider{
		{
			Key:   "github",
			Label: "GitHub",
			Fields: []CredField{
				{Key: "github.oauth_client_id", Label: "OAuth client id", Cur: func(a *app.App) string { return a.Cfg.GitHub.OAuthClientID }},
			},
			Authed: func(a *app.App) bool { return a.GitHubAuthed() },
			Login: func(ctx context.Context, a *app.App, creds map[string]string, w io.Writer) error {
				id := eff(creds, "github.oauth_client_id", a.Cfg.GitHub.OAuthClientID)
				return auth.LoginGitHub(ctx, a.Tokens, id, a.Cfg.GitHub.OAuthScopes, w)
			},
		},
		{
			Key:     "google",
			Label:   "Google",
			Signals: []string{"calendar", "gmail", "docs", "drive", "tasks"},
			Fields: []CredField{
				{Key: "google.oauth_client_id", Label: "OAuth client id", Cur: func(a *app.App) string { return a.Cfg.Google.OAuthClientID }},
				{Key: "google.oauth_client_secret", Label: "OAuth client secret", Secret: true, Cur: func(a *app.App) string { return a.Cfg.Google.OAuthClientSecret }},
			},
			Authed: func(a *app.App) bool { return auth.GoogleAuthed(a.Tokens) },
			Login: func(ctx context.Context, a *app.App, creds map[string]string, w io.Writer) error {
				return auth.GoogleLogin(ctx, auth.GoogleAuth{
					Store:        a.Tokens,
					ClientID:     eff(creds, "google.oauth_client_id", a.Cfg.Google.OAuthClientID),
					ClientSecret: eff(creds, "google.oauth_client_secret", a.Cfg.Google.OAuthClientSecret),
				}, w)
			},
		},
		{
			Key:   "slack",
			Label: "Slack",
			Fields: []CredField{
				{Key: "slack.oauth_client_id", Label: "OAuth client id", Cur: func(a *app.App) string { return a.Cfg.Slack.OAuthClientID }},
				{Key: "slack.oauth_client_secret", Label: "OAuth client secret", Secret: true, Cur: func(a *app.App) string { return a.Cfg.Slack.OAuthClientSecret }},
			},
			Authed: func(a *app.App) bool { _, err := auth.SlackToken(a.Tokens, a.Cfg.Slack.TokenEnv); return err == nil },
			Login: func(ctx context.Context, a *app.App, creds map[string]string, w io.Writer) error {
				return auth.LoginSlack(ctx, a.Tokens,
					eff(creds, "slack.oauth_client_id", a.Cfg.Slack.OAuthClientID),
					eff(creds, "slack.oauth_client_secret", a.Cfg.Slack.OAuthClientSecret),
					a.Cfg.Slack.UserScopes, w)
			},
		},
	}
}

func Resolve(name string) (Provider, bool) {
	for _, p := range Providers() {
		if p.Key == name || slices.Contains(p.Signals, name) {
			return p, true
		}
	}
	return Provider{}, false
}

func Names() []string {
	var out []string
	for _, p := range Providers() {
		out = append(out, p.Key)
		out = append(out, p.Signals...)
	}
	return out
}

func PersistCredentials(a *app.App, values map[string]string) error {
	if len(values) == 0 {
		return nil
	}
	doc := make(map[string]any, len(values))
	for k, v := range values {
		doc[k] = v
	}
	_, err := config.SetValues(a.Cfg.Home, doc)
	return err
}

func eff(creds map[string]string, key, cur string) string {
	if v := creds[key]; v != "" {
		return v
	}
	return cur
}
