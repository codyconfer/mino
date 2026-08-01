package build

import (
	"context"

	"github.com/codyconfer/sisyphus/daemon"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/plugin"
	"github.com/codyconfer/mino/internal/pluginhost"
	"github.com/codyconfer/mino/internal/signals/active"
	"github.com/codyconfer/mino/internal/signals/cache"
	"github.com/codyconfer/mino/internal/token"
)

type hostBuildCtx struct {
	signal string
	params map[string]string
	cfg    *config.Config
	tokens *token.Store
	state  *active.State
	cache  *cache.Store
	grant  pluginhost.Grant
}

func newHostBuildCtx(signal string, params map[string]string, cfg *config.Config, tokens *token.Store, state *active.State, results *cache.Store) hostBuildCtx {
	return hostBuildCtx{
		signal: signal,
		params: params,
		cfg:    cfg,
		tokens: tokens,
		state:  state,
		cache:  results,
		grant:  pluginhost.GrantForSignal(signal),
	}
}

func (c hostBuildCtx) Params() map[string]string {
	out := make(map[string]string, len(c.params))
	for k, v := range c.params {
		out[k] = v
	}
	return out
}

func (c hostBuildCtx) Home() string {
	if c.cfg == nil {
		return ""
	}
	return c.cfg.Home
}

func (c hostBuildCtx) Role() string {
	if c.cfg == nil {
		return ""
	}
	return c.cfg.Role
}

func (c hostBuildCtx) Settings(namespace string) map[string]any {
	if !c.grant.AllowsNamespace(namespace) {
		plugin.NoteDiagnostic(c.grant.Owner, "", namespace,
			"read settings namespace "+namespace+" without declaring it in Descriptor.SettingsNamespaces; returned no settings")
		return nil
	}
	return c.cfg.PluginSettings(namespace)
}

func (c hostBuildCtx) Credentials() plugin.CredentialStore {
	if c.tokens == nil {
		return nil
	}
	return pluginhost.ScopeCredentials(c.tokens, c.grant)
}

func (c hostBuildCtx) KV() daemon.KV {
	return plugin.ScopeKV(c.state.KV(), kvOwner(c.signal))
}

func kvOwner(signal string) string {
	if d, ok := plugin.BySignal(signal); ok {
		return plugin.OwnerID(d)
	}
	return signal
}

func (c hostBuildCtx) GetToken(ctx context.Context, service string) (accessToken, scope string, ok bool, err error) {
	if err := c.grant.CheckCredential(service); err != nil {
		return "", "", false, err
	}
	if c.tokens == nil {
		return "", "", false, nil
	}
	cred, found, err := c.tokens.Get(ctx, service)
	if err != nil || !found {
		return "", "", found, err
	}
	return cred.AccessToken, cred.Scope, true, nil
}

func asHost(bc plugin.BuildContext) (hostBuildCtx, bool) {
	h, ok := bc.(hostBuildCtx)
	return h, ok
}
