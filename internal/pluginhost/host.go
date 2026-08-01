package pluginhost

import (
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/token"
	"github.com/codyconfer/mino/plugin"
)

type host struct {
	cfg    *config.Config
	tokens *token.Store
}

func New(cfg *config.Config, tokens *token.Store) plugin.Host {
	return host{cfg: cfg, tokens: tokens}
}

func (h host) Home() string {
	if h.cfg == nil {
		return ""
	}
	return h.cfg.Home
}

func (h host) Role() string {
	if h.cfg == nil {
		return ""
	}
	return h.cfg.Role
}

func (h host) Settings(namespace string) map[string]any {
	return h.cfg.PluginSettings(namespace)
}

func (h host) Credentials() plugin.CredentialStore {
	if h.tokens == nil {
		return nil
	}
	return h.tokens
}
