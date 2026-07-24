package loginflow

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/config"
)

func TestResolveAliases(t *testing.T) {
	cases := map[string]string{
		"github":   "github",
		"google":   "google",
		"gmail":    "google",
		"calendar": "google",
		"docs":     "google",
		"drive":    "google",
		"tasks":    "google",
		"slack":    "slack",
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

func TestMissing(t *testing.T) {
	cfg := config.Defaults()
	cfg.Google.OAuthClientID = "id-only"
	a := &app.App{Cfg: cfg}

	p, _ := Resolve("google")
	miss := p.Missing(a)
	if len(miss) != 1 || miss[0].Key != "google.oauth_client_secret" {
		t.Fatalf("expected only client secret missing, got %#v", miss)
	}

	cfg.Google.OAuthClientSecret = "secret"
	if m := p.Missing(a); len(m) != 0 {
		t.Fatalf("expected nothing missing, got %#v", m)
	}
}

func TestPersistCredentials(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Defaults()
	cfg.Home = dir
	a := &app.App{Cfg: cfg}

	if err := PersistCredentials(a, map[string]string{
		"google.oauth_client_id":     "abc",
		"google.oauth_client_secret": "xyz",
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
	g, ok := got["google"].(map[string]any)
	if !ok || g["oauth_client_id"] != "abc" || g["oauth_client_secret"] != "xyz" {
		t.Fatalf("credentials not written: %#v", got["google"])
	}
}
