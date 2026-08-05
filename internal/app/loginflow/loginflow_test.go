package loginflow

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/plugin"
)

func registerFakeGoogle(t *testing.T) {
	t.Helper()
	plugin.ResetLoginProviders()
	t.Cleanup(plugin.ResetLoginProviders)
	plugin.RegisterLoginProvider(plugin.LoginProvider{
		PluginID: "external.google",
		Key:      "google",
		Label:    "Google",
		Signals:  []string{"calendar", "gmail", "docs", "drive", "tasks"},
		Fields: []plugin.LoginField{
			{Key: "plugins.google.oauth_client_id", Label: "OAuth client id", Value: setting("oauth_client_id")},
			{Key: "plugins.google.oauth_client_secret", Label: "OAuth client secret", Secret: true, Value: setting("oauth_client_secret")},
		},
		Authed: func(plugin.Host) bool { return false },
		Login:  func(context.Context, plugin.Host, map[string]string, io.Writer) error { return nil },
	})
}

func setting(key string) func(plugin.Host) string {
	return func(h plugin.Host) string {
		if h == nil {
			return ""
		}
		return plugin.Setting(h.Settings("google"), key, "")
	}
}

func TestResolveAliases(t *testing.T) {
	registerFakeGoogle(t)
	cases := map[string]string{
		"github":   "github",
		"google":   "google",
		"gmail":    "google",
		"calendar": "google",
		"docs":     "google",
		"drive":    "google",
		"tasks":    "google",
	}
	for name, want := range cases {
		p, ok := Resolve(name)
		if !ok {
			t.Errorf("Resolve(%q) not found", name)
			continue
		}
		if p.Key != want {
			t.Errorf("Resolve(%q) = %q, want %q", name, p.Key, want)
		}
	}
	if _, ok := Resolve("nope"); ok {
		t.Error("Resolve(nope) should be false")
	}
}

func TestContributedProviderIsNotResolvedWhenUnregistered(t *testing.T) {
	plugin.ResetLoginProviders()
	t.Cleanup(plugin.ResetLoginProviders)
	if _, ok := Resolve("google"); ok {
		t.Error("google resolves without a plugin: stock mino no longer ships the Google signals")
	}
}

func TestMissingReadsContributedFieldsFromPluginSettings(t *testing.T) {
	registerFakeGoogle(t)
	cfg := config.Defaults()
	cfg.Plugins = map[string]map[string]any{"google": {"oauth_client_id": "id-only"}}
	a := &app.App{Cfg: cfg}
	t.Cleanup(a.CloseDBs)

	p, _ := Resolve("google")
	miss := p.Missing(a)
	if len(miss) != 1 || miss[0].Key != "plugins.google.oauth_client_secret" {
		t.Fatalf("expected only client secret missing, got %#v", miss)
	}

	cfg.Plugins["google"]["oauth_client_secret"] = "secret"
	if m := p.Missing(a); len(m) != 0 {
		t.Fatalf("expected nothing missing, got %#v", m)
	}
}

func TestPersistCredentials(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Defaults()
	cfg.Home = dir
	a := &app.App{Cfg: cfg}
	t.Cleanup(a.CloseDBs)

	if err := PersistCredentials(a, map[string]string{
		"plugins.google.oauth_client_id":     "abc",
		"plugins.google.oauth_client_secret": "xyz",
	}); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]any{}
	if err := yaml.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	plugins, ok := got["plugins"].(map[string]any)
	if !ok {
		t.Fatalf("credentials not written under plugins: %#v", got)
	}
	g, ok := plugins["google"].(map[string]any)
	if !ok || g["oauth_client_id"] != "abc" || g["oauth_client_secret"] != "xyz" {
		t.Fatalf("credentials not written: %#v", plugins["google"])
	}
}
