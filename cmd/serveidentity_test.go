package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/config"
)

func useIdentityTestApp(t *testing.T, mut func(*config.Config)) {
	t.Helper()
	orig := shared
	t.Cleanup(func() { shared = orig })
	cfg := config.Defaults()
	cfg.Home = t.TempDir()
	cfg.Daemon.HTTP.Enabled = true
	cfg.Daemon.HTTP.Port = config.DefaultHTTPPort
	cfg.Daemon.HTTP.Identity.Enabled = true
	cfg.Daemon.HTTP.Identity.ClientID = "Ov23liExampleClientId"
	cfg.Daemon.HTTP.Identity.AllowedLogins = []string{"codyconfer"}
	if mut != nil {
		mut(cfg)
	}
	shared = &app.App{Cfg: cfg, Directives: &config.Directives{}}
}

func TestIdentityLoginResolvesFromConfig(t *testing.T) {
	useIdentityTestApp(t, nil)
	got, err := resolveServeHTTPIdentity()
	if err != nil {
		t.Fatalf("resolveServeHTTPIdentity: %v", err)
	}
	if !got.Enabled || got.Provider != "github" {
		t.Errorf("got %+v, want enabled github", got)
	}
	if got.SessionTTL != 12*time.Hour {
		t.Errorf("session ttl = %s, want the 12h default", got.SessionTTL)
	}
	if got.Scopes != "" {
		t.Errorf("scopes = %q, want empty; resolving a login needs no scope, and asking for repo "+
			"access to trigger a flight is indefensible", got.Scopes)
	}
	if got.DeviceURL == "" || got.TokenURL == "" {
		t.Error("the device endpoints were not resolved")
	}
}

func TestIdentityLoginIsOffByDefault(t *testing.T) {
	useServeHTTPTestApp(t, true, config.DefaultHTTPPort)
	got, err := resolveServeHTTPIdentity()
	if err != nil {
		t.Fatalf("resolveServeHTTPIdentity: %v", err)
	}
	if got.Enabled {
		t.Error("identity login was on with nothing configured; it must be opt-in")
	}
}

func TestServeRefusesIdentityLoginWithAnEmptyAllowList(t *testing.T) {
	useIdentityTestApp(t, func(c *config.Config) {
		c.Daemon.HTTP.Identity.AllowedLogins = nil
	})
	err := runServe(t)
	if err == nil {
		t.Fatal("serve with an empty allow-list = nil; read as \"allow all\" that would let any account " +
			"on the forge trigger flights, run queries and execute plugin actions here")
	}
	if !strings.Contains(err.Error(), "daemon.http.identity.allowed_logins") {
		t.Errorf("error = %q; want daemon.http.identity.allowed_logins named", err)
	}
}

func TestServeRefusesIdentityLoginWithNoClientID(t *testing.T) {
	useIdentityTestApp(t, func(c *config.Config) {
		c.Daemon.HTTP.Identity.ClientID = ""
		c.GitHub.OAuthClientID = ""
	})
	err := runServe(t)
	if err == nil {
		t.Fatal("serve with no client id = nil; the device flow cannot start without one")
	}
	for _, want := range []string{"daemon.http.identity.client_id", "github.oauth_client_id"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q; want %s named so the user knows either key will do", err, want)
		}
	}
}

func TestIdentityLoginFallsBackToTheGitHubClientID(t *testing.T) {
	useIdentityTestApp(t, func(c *config.Config) {
		c.Daemon.HTTP.Identity.ClientID = ""
		c.GitHub.OAuthClientID = "Ov23liFromGitHubBlock"
	})
	got, err := resolveServeHTTPIdentity()
	if err != nil {
		t.Fatalf("resolveServeHTTPIdentity: %v", err)
	}
	if got.ClientID != "Ov23liFromGitHubBlock" {
		t.Errorf("client id = %q, want the github.oauth_client_id fallback", got.ClientID)
	}
}

func TestServeRefusesAnUnknownIdentityProvider(t *testing.T) {
	useIdentityTestApp(t, func(c *config.Config) {
		c.Daemon.HTTP.Identity.Provider = "gitlab"
	})
	err := runServe(t)
	if err == nil {
		t.Fatal("serve with an unknown provider = nil; it would boot with sign-in silently dead")
	}
	if !strings.Contains(err.Error(), "daemon.http.identity.provider") {
		t.Errorf("error = %q; want daemon.http.identity.provider named", err)
	}
}

func TestServeRefusesAMalformedAllowedLogin(t *testing.T) {
	for _, bad := range []string{"@cody", "cody confer", "-cody", "cody/confer", strings.Repeat("c", 40)} {
		useIdentityTestApp(t, func(c *config.Config) {
			c.Daemon.HTTP.Identity.AllowedLogins = []string{bad}
		})
		err := runServe(t)
		if err == nil {
			t.Errorf("serve with allowed_logins %q = nil; an entry that can never match leaves the "+
				"allow-list silently denying everyone", bad)
			continue
		}
		if !strings.Contains(err.Error(), "allowed_logins") {
			t.Errorf("error for %q = %q; want allowed_logins named", bad, err)
		}
	}
}

func TestServeRefusesADuplicateAllowedLogin(t *testing.T) {
	useIdentityTestApp(t, func(c *config.Config) {
		c.Daemon.HTTP.Identity.AllowedLogins = []string{"Cody", "cody"}
	})
	err := runServe(t)
	if err == nil {
		t.Fatal("serve with a case-duplicate allow-list = nil; logins are case-insensitive, so one of " +
			"those entries is dead weight the operator should know about")
	}
}

func TestServeRefusesAnOutOfRangeSessionTTL(t *testing.T) {
	for _, tc := range []struct{ name, ttl string }{
		{"unparseable", "twelve hours"},
		{"too short", "1s"},
		{"too long", "87600h"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			useIdentityTestApp(t, func(c *config.Config) {
				c.Daemon.HTTP.Identity.SessionTTL = tc.ttl
			})
			err := runServe(t)
			if err == nil {
				t.Fatalf("serve with session_ttl %q = nil", tc.ttl)
			}
			if !strings.Contains(err.Error(), "daemon.http.identity.session_ttl") {
				t.Errorf("error = %q; want daemon.http.identity.session_ttl named", err)
			}
		})
	}
}

func TestServeIgnoresTheIdentityBlockWhenTheAPIIsOff(t *testing.T) {
	useIdentityTestApp(t, func(c *config.Config) {
		c.Daemon.HTTP.Enabled = false
		c.Daemon.HTTP.Identity.AllowedLogins = nil
	})
	if err := runServe(t, "--http=false"); err != nil && strings.Contains(err.Error(), "identity") {
		t.Errorf("serve --http=false rejected a stale identity block: %v; validating it "+
			"unconditionally would break every serve invocation the moment one sat in config", err)
	}
}

func TestIdentityLoginRefusesACleartextAPIURL(t *testing.T) {
	useIdentityTestApp(t, func(c *config.Config) {
		c.GitHub.APIURL = "http://ghe.example.com/api/v3"
	})
	err := runServe(t)
	if err == nil {
		t.Fatal("serve with a cleartext api_url = nil; the device code and the access token both cross it")
	}
}
