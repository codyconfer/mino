package pluginhost

import (
	"context"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/token"
	"github.com/codyconfer/mino/plugin"
)

type Grant struct {
	Owner       string
	credentials map[string]bool
	namespaces  map[string]bool
}

func GrantForSignal(signal string) Grant {
	d, ok := plugin.BySignal(signal)
	if !ok {
		return newGrant(signal, signal, nil, nil)
	}
	return newGrant(plugin.OwnerID(d), signal, d.Credentials, d.SettingsNamespaces)
}

func GrantFor(ownerID, fallback string) Grant {
	d, ok := plugin.Lookup(ownerID)
	if !ok {
		own := fallback
		if own == "" {
			own = ownerID
		}
		return newGrant(ownerID, own, nil, nil)
	}
	own := d.Signal
	if own == "" {
		own = fallback
	}
	if own == "" {
		own = d.Ref
	}
	if own == "" {
		own = ownerID
	}
	return newGrant(ownerID, own, d.Credentials, d.SettingsNamespaces)
}

func newGrant(owner, own string, credentials, namespaces []string) Grant {
	if len(credentials) == 0 {
		credentials = []string{own}
	}
	if len(namespaces) == 0 {
		namespaces = []string{own}
	}
	return Grant{Owner: owner, credentials: toSet(credentials), namespaces: toSet(namespaces)}
}

func toSet(names []string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, name := range names {
		out[name] = true
	}
	return out
}

func (g Grant) AllowsNamespace(ns string) bool { return g.namespaces[ns] }

func (g Grant) CheckCredential(service string) error {
	if g.credentials[service] {
		return nil
	}
	return errs.Newf(errs.KindAuth, "plugin %q may not access credential service %q", g.Owner, service).
		WithHint("declare the service in the plugin's Descriptor.Credentials")
}

func ScopeCredentials(store plugin.CredentialStore, g Grant) plugin.CredentialStore {
	if store == nil {
		return nil
	}
	return scopedCredentials{store: store, grant: g}
}

type scopedCredentials struct {
	store plugin.CredentialStore
	grant Grant
}

func (s scopedCredentials) Get(ctx context.Context, service string) (plugin.Credential, bool, error) {
	if err := s.grant.CheckCredential(service); err != nil {
		return plugin.Credential{}, false, err
	}
	return s.store.Get(ctx, service)
}

func (s scopedCredentials) Put(ctx context.Context, service string, c plugin.Credential) error {
	if err := s.grant.CheckCredential(service); err != nil {
		return err
	}
	return s.store.Put(ctx, service, c)
}

func (s scopedCredentials) Delete(ctx context.Context, service string) error {
	if err := s.grant.CheckCredential(service); err != nil {
		return err
	}
	return s.store.Delete(ctx, service)
}

type host struct {
	cfg    *config.Config
	tokens *token.Store
	role   string
	grant  Grant
}

func ForSignal(cfg *config.Config, tokens *token.Store, role, signal string) plugin.Host {
	return host{cfg: cfg, tokens: tokens, role: role, grant: GrantForSignal(signal)}
}

func ForPlugin(cfg *config.Config, tokens *token.Store, role, pluginID, fallback string) plugin.Host {
	return host{cfg: cfg, tokens: tokens, role: role, grant: GrantFor(pluginID, fallback)}
}

func ForLogin(cfg *config.Config, tokens *token.Store, role string, p plugin.LoginProvider) plugin.Host {
	return host{cfg: cfg, tokens: tokens, role: role, grant: GrantFor(p.PluginID, p.Key)}
}

func (h host) Home() string {
	if h.cfg == nil {
		return ""
	}
	return h.cfg.Home
}

func (h host) Role() string { return h.role }

func (h host) Settings(namespace string) map[string]any {
	if !h.grant.AllowsNamespace(namespace) {
		plugin.NoteDiagnostic(h.grant.Owner, "", namespace,
			"read settings namespace "+namespace+" without declaring it in Descriptor.SettingsNamespaces; returned no settings")
		return nil
	}
	return h.cfg.PluginSettings(namespace)
}

func (h host) Credentials() plugin.CredentialStore {
	if h.tokens == nil {
		return nil
	}
	return ScopeCredentials(h.tokens, h.grant)
}
