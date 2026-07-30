package build

import (
	"context"

	"github.com/codyconfer/sisyphus/daemon"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/signals/active"
	"github.com/codyconfer/munin/internal/signals/cache"
	"github.com/codyconfer/munin/internal/token"
)

type hostBuildCtx struct {
	signal string
	params map[string]string
	cfg    *config.Config
	tokens *token.Store
	state  *active.State
	cache  *cache.Store
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

// KV hands the builder a view of the serve key/value store confined to the
// owning plugin's own namespace prefix. The raw daemon.KV is read, write and
// delete over every namespace, so handing it out unprefixed would let any plugin
// (including an external one, which reaches this through a
// `bc.(interface{ KV() daemon.KV })` assertion) read or wipe another signal's
// persisted cursors. First-party builders inside this package keep the raw
// handle through the unexported state field, which no other module can name.
func (c hostBuildCtx) KV() daemon.KV {
	return plugin.ScopeKV(c.state.KV(), kvOwner(c.signal))
}

func kvOwner(signal string) string {
	if d, ok := plugin.BySignal(signal); ok {
		return plugin.OwnerID(d)
	}
	return signal
}

// GetToken is unscoped by design and predates the plugin SDK: a builder asks for
// a service credential by name. Any tightening belongs with the token store.
func (c hostBuildCtx) GetToken(ctx context.Context, service string) (accessToken, scope string, ok bool, err error) {
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
