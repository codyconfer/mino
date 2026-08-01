package loginflow

import (
	"context"
	"io"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/plugin"
	"github.com/codyconfer/mino/internal/pluginhost"
)

func registered() []Provider {
	contributed := plugin.LoginProviders()
	out := make([]Provider, 0, len(contributed))
	for _, p := range contributed {
		out = append(out, adapt(p))
	}
	return out
}

func adapt(p plugin.LoginProvider) Provider {
	fields := make([]CredField, 0, len(p.Fields))
	for _, f := range p.Fields {
		fields = append(fields, CredField{
			Key:    f.Key,
			Label:  f.Label,
			Secret: f.Secret,
			Cur:    currentValue(p, f),
		})
	}
	return Provider{
		Key:     p.Key,
		Label:   p.Label,
		Signals: p.Signals,
		Fields:  fields,
		Authed: func(a *app.App) bool {
			if p.Authed == nil {
				return false
			}
			return p.Authed(hostFor(a, p))
		},
		Login: func(ctx context.Context, a *app.App, creds map[string]string, w io.Writer) error {
			return p.Login(ctx, hostFor(a, p), creds, w)
		},
	}
}

func currentValue(p plugin.LoginProvider, f plugin.LoginField) func(*app.App) string {
	return func(a *app.App) string {
		if f.Value == nil {
			return ""
		}
		return f.Value(hostFor(a, p))
	}
}

func hostFor(a *app.App, p plugin.LoginProvider) plugin.Host {
	if a == nil {
		return pluginhost.ForLogin(nil, nil, p)
	}
	return pluginhost.ForLogin(a.Cfg, a.Tokens, p)
}
