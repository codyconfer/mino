package loginflow

import (
	"context"
	"io"
	"slices"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/auth"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/gitauth"
)

type CredField struct {
	Key    string
	Label  string
	Cur    func(*app.App) string
	Secret bool
	Sealed bool
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

func (p Provider) Persistable(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	sealed := make(map[string]bool, len(p.Fields))
	for _, f := range p.Fields {
		if f.Sealed {
			sealed[f.Key] = true
		}
	}
	out := make(map[string]string, len(values))
	for k, v := range values {
		if sealed[k] {
			continue
		}
		out[k] = v
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
			Authed: gitProviderAuthed("github", "github"),
			Login: func(ctx context.Context, a *app.App, creds map[string]string, w io.Writer) error {
				id := eff(creds, "github.oauth_client_id", a.Cfg.GitHub.OAuthClientID)
				return auth.LoginGitHub(ctx, a.Tokens, id, a.Cfg.GitHub.OAuthScopes, w)
			},
		},
		{
			Key:     "gitea",
			Label:   "Gitea",
			Signals: []string{"forgejo", "gitea"},
			Fields: []CredField{{
				Key:    "gitea.token",
				Label:  "Personal access token (scopes: read:user, read:repository)",
				Secret: true,
				Sealed: true,
				Cur:    func(a *app.App) string { return auth.GiteaTokenOrigin(a.Tokens) },
			}},
			Authed: func(a *app.App) bool { return auth.GiteaAuthed(giteaSpec(a)) },
			Login: func(ctx context.Context, a *app.App, creds map[string]string, w io.Writer) error {
				return auth.LoginGitea(ctx, a.Tokens, giteaSpec(a), creds["gitea.token"], w)
			},
		},
		{
			Key:   "gitlab",
			Label: "GitLab",
			Fields: []CredField{
				{Key: "gitlab.oauth_client_id", Label: "OAuth application id", Cur: func(a *app.App) string { return a.Cfg.GitLab.OAuthClientID }},
			},
			Authed: gitProviderAuthed("gitlab", "gitlab"),
			Login: func(ctx context.Context, a *app.App, creds map[string]string, w io.Writer) error {
				id := eff(creds, "gitlab.oauth_client_id", a.Cfg.GitLab.OAuthClientID)
				return auth.LoginGitLab(ctx, a.Tokens, a.Cfg.GitLab.APIURL, id, a.Cfg.GitLab.OAuthScopes, w)
			},
		},
	}
}

func giteaSpec(a *app.App) auth.GiteaSpec {
	if a == nil || a.Cfg == nil {
		return auth.GiteaSpec{}
	}
	forge := a.Cfg.GitProvider()
	if forge != "forgejo" {
		forge = "gitea"
	}
	return a.Cfg.Gitea.AuthSpec(forge, a.Tokens)
}

func gitProviderAuthed(name, service string) func(*app.App) bool {
	return func(a *app.App) bool {
		if ActiveGitProvider(a) == name {
			return a.GitAuthed()
		}
		if a == nil || a.Tokens == nil {
			return false
		}
		_, state, _ := auth.ReadCredential(a.Tokens, service)
		return state == auth.CredValid
	}
}

func ActiveGitProvider(a *app.App) string {
	if a == nil || a.Cfg == nil {
		return gitauth.Default
	}
	if n := a.Cfg.GitProvider(); n != "" {
		return n
	}
	return gitauth.Default
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
