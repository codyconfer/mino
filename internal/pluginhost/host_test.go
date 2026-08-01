package pluginhost_test

import (
	"context"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/pluginhost"
	"github.com/codyconfer/mino/plugin"
)

type fakeStore struct {
	gets    []string
	puts    []string
	deletes []string
}

func (f *fakeStore) Get(_ context.Context, service string) (plugin.Credential, bool, error) {
	f.gets = append(f.gets, service)
	return plugin.Credential{AccessToken: "tok-" + service}, true, nil
}

func (f *fakeStore) Put(_ context.Context, service string, _ plugin.Credential) error {
	f.puts = append(f.puts, service)
	return nil
}

func (f *fakeStore) Delete(_ context.Context, service string) error {
	f.deletes = append(f.deletes, service)
	return nil
}

func TestGrantForSignalDefaultsToTheSignalName(t *testing.T) {
	plugin.Register(plugin.Descriptor{
		ID:     "test.grant.defaults",
		Kind:   plugin.KindSignal,
		Signal: "grantdefaults",
	})
	g := pluginhost.GrantForSignal("grantdefaults")
	if !g.AllowsNamespace("grantdefaults") {
		t.Error("a plugin must read its own settings namespace by default")
	}
	if g.AllowsNamespace("google") {
		t.Error("an undeclared namespace must not be granted")
	}
	if err := g.CheckCredential("grantdefaults"); err != nil {
		t.Errorf("CheckCredential(own) = %v, want nil", err)
	}
	err := g.CheckCredential("google")
	if err == nil {
		t.Fatal("CheckCredential for an undeclared service must error")
	}
	if errs.KindOf(err) != errs.KindAuth {
		t.Errorf("kind = %v, want %v", errs.KindOf(err), errs.KindAuth)
	}
}

func TestGrantExplicitListsReplaceTheDefaults(t *testing.T) {
	plugin.Register(plugin.Descriptor{
		ID:                 "test.grant.explicit",
		Kind:               plugin.KindSignal,
		Signal:             "grantexplicit",
		Credentials:        []string{"google"},
		SettingsNamespaces: []string{"grantexplicit", "google"},
	})
	g := pluginhost.GrantForSignal("grantexplicit")
	if err := g.CheckCredential("google"); err != nil {
		t.Errorf("declared credential refused: %v", err)
	}
	if err := g.CheckCredential("grantexplicit"); err == nil {
		t.Error("an explicit Credentials list must replace the own-name default, not extend it")
	}
	if !g.AllowsNamespace("grantexplicit") || !g.AllowsNamespace("google") {
		t.Error("declared namespaces refused")
	}
	if g.AllowsNamespace("slack") {
		t.Error("an undeclared namespace must not be granted")
	}
}

func TestGrantForUnregisteredOwnerUsesTheFallback(t *testing.T) {
	g := pluginhost.GrantFor("test.grant.unregistered", "fallbackname")
	if err := g.CheckCredential("fallbackname"); err != nil {
		t.Errorf("CheckCredential(fallback) = %v, want nil", err)
	}
	if err := g.CheckCredential("other"); err == nil {
		t.Error("an undeclared service must be refused for unregistered owners too")
	}
	if !g.AllowsNamespace("fallbackname") || g.AllowsNamespace("other") {
		t.Error("unregistered owner namespace grant should be exactly the fallback name")
	}
}

func TestScopeCredentialsBlocksForeignServicesWithoutTouchingTheStore(t *testing.T) {
	ctx := context.Background()
	store := &fakeStore{}
	scoped := pluginhost.ScopeCredentials(store, pluginhost.GrantFor("test.grant.scoped", "own"))

	if _, ok, err := scoped.Get(ctx, "own"); err != nil || !ok {
		t.Fatalf("Get(own) = ok %v err %v, want the underlying store's answer", ok, err)
	}
	if err := scoped.Put(ctx, "own", plugin.Credential{}); err != nil {
		t.Fatalf("Put(own) = %v", err)
	}
	if err := scoped.Delete(ctx, "own"); err != nil {
		t.Fatalf("Delete(own) = %v", err)
	}

	if _, ok, err := scoped.Get(ctx, "github"); err == nil || ok {
		t.Error("Get for a foreign service must error")
	}
	if err := scoped.Put(ctx, "github", plugin.Credential{}); err == nil {
		t.Error("Put for a foreign service must error")
	}
	if err := scoped.Delete(ctx, "github"); err == nil {
		t.Error("Delete for a foreign service must error")
	}

	for name, calls := range map[string][]string{"Get": store.gets, "Put": store.puts, "Delete": store.deletes} {
		if len(calls) != 1 || calls[0] != "own" {
			t.Errorf("%s reached the underlying store on deny: calls = %v", name, calls)
		}
	}
}

func TestScopeCredentialsNilStoreStaysNil(t *testing.T) {
	if got := pluginhost.ScopeCredentials(nil, pluginhost.GrantFor("x", "y")); got != nil {
		t.Fatalf("ScopeCredentials(nil) = %v, want nil", got)
	}
}

func TestForSignalScopesSettingsAndNotesADiagnostic(t *testing.T) {
	plugin.Register(plugin.Descriptor{
		ID:     "test.grant.settings",
		Kind:   plugin.KindSignal,
		Signal: "grantsettings",
	})
	cfg := &config.Config{Plugins: map[string]map[string]any{
		"grantsettings": {"max": "3"},
		"google":        {"oauth_client_id": "abc"},
	}}
	h := pluginhost.ForSignal(cfg, nil, "grantsettings")
	if got := plugin.Setting(h.Settings("grantsettings"), "max", ""); got != "3" {
		t.Errorf("own settings = %q, want 3", got)
	}
	if got := h.Settings("google"); got != nil {
		t.Errorf("foreign settings leaked: %v", got)
	}
	found := false
	for _, d := range plugin.DiagnosticsFor("test.grant.settings") {
		if strings.Contains(d.Message, "google") {
			found = true
		}
	}
	if !found {
		t.Error("a settings violation must record a plugin diagnostic")
	}
	if h.Credentials() != nil {
		t.Error("nil token store must yield nil Credentials")
	}
}

func TestForLoginGrantsTheProviderKey(t *testing.T) {
	p := plugin.LoginProvider{PluginID: "test.grant.loginplugin", Key: "grantlogin"}
	cfg := &config.Config{Plugins: map[string]map[string]any{
		"grantlogin": {"oauth_client_id": "abc"},
		"slack":      {"limit": "9"},
	}}
	h := pluginhost.ForLogin(cfg, nil, p)
	if got := plugin.Setting(h.Settings("grantlogin"), "oauth_client_id", ""); got != "abc" {
		t.Errorf("own settings = %q, want abc", got)
	}
	if got := h.Settings("slack"); got != nil {
		t.Errorf("foreign settings leaked: %v", got)
	}
}
