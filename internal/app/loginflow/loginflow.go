package loginflow

import (
	"context"
	"io"
	"slices"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/auth"
	"github.com/codyconfer/mino/internal/config"
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
	return append(coreProviders(), registered()...)
}

func coreProviders() []Provider {
	return []Provider{
		{
			Key:   "github",
			Label: "GitHub",
			Fields: []CredField{
				{Key: "github.oauth_client_id", Label: "OAuth client id", Cur: func(a *app.App) string { return a.Cfg.GitHub.OAuthClientID }},
			},
			Authed: func(a *app.App) bool { return a.GitAuthed() },
			Login: func(ctx context.Context, a *app.App, creds map[string]string, w io.Writer) error {
				id := eff(creds, "github.oauth_client_id", a.Cfg.GitHub.OAuthClientID)
				return auth.LoginGitHub(ctx, a.Tokens, id, a.Cfg.GitHub.OAuthScopes, w)
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
