package build

import (
	"context"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/signals/active"
	"github.com/codyconfer/munin/internal/signals/cache"
	"github.com/codyconfer/munin/internal/token"
)

type hostBuildCtx struct {
	params map[string]string
	cfg    *config.Config
	tokens *token.Store
	state  *active.State
	cache  *cache.Store
}

func (c hostBuildCtx) Params() map[string]string {
	if c.params == nil {
		return map[string]string{}
	}
	return c.params
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
