package config

import (
	"slices"
	"testing"
)

func TestListValuesSplitsCommasAndTrims(t *testing.T) {
	got := ListValues([]string{" alice , bob ", "", "carol", "bob"})
	want := []string{"alice", "bob", "carol"}
	if !slices.Equal(got, want) {
		t.Errorf("ListValues = %v, want %v", got, want)
	}
}

func TestAllowedLoginsFromEnvAndYAMLAgree(t *testing.T) {
	yaml := HTTPIdentityConfig{AllowedLogins: []string{"alice", "bob"}}
	fromEnv := HTTPIdentityConfig{AllowedLogins: []string{"alice,bob"}}
	if !slices.Equal(yaml.LoginNames(), fromEnv.LoginNames()) {
		t.Errorf("yaml = %v but env = %v; a MINO_* override can only set one string, and koanf lifts "+
			"that into a one-element slice rather than splitting it, so without normalizing, a container "+
			"gets an allow-list of one entry that matches nobody",
			yaml.LoginNames(), fromEnv.LoginNames())
	}
}

func TestIdentityEnvOverridesApply(t *testing.T) {
	t.Setenv("MINO_DAEMON_HTTP_IDENTITY_ENABLED", "true")
	t.Setenv("MINO_DAEMON_HTTP_IDENTITY_CLIENT_ID", "Ov23liFromEnv")
	t.Setenv("MINO_DAEMON_HTTP_IDENTITY_SESSION_TTL", "6h")
	t.Setenv("MINO_DAEMON_HTTP_IDENTITY_ALLOWED_LOGINS", "alice,bob")

	cfg, err := ParseConfig(t.TempDir(), nil, "yaml")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	id := cfg.Daemon.HTTP.Identity
	if !id.Enabled {
		t.Error("MINO_DAEMON_HTTP_IDENTITY_ENABLED did not apply")
	}
	if id.ClientID != "Ov23liFromEnv" {
		t.Errorf("client id = %q, want the env value", id.ClientID)
	}
	if id.SessionTTL != "6h" {
		t.Errorf("session ttl = %q, want 6h", id.SessionTTL)
	}
	if got := id.LoginNames(); !slices.Equal(got, []string{"alice", "bob"}) {
		t.Errorf("allowed logins = %v, want [alice bob]", got)
	}
}

func TestDefaultsLeaveIdentityLoginOff(t *testing.T) {
	id := Defaults().Daemon.HTTP.Identity
	if id.Active() {
		t.Error("identity login is on by default; it must be opt-in")
	}
	if id.ProviderName() != DefaultHTTPIdentityProvider {
		t.Errorf("provider = %q, want %q", id.ProviderName(), DefaultHTTPIdentityProvider)
	}
	if id.Scopes != "" {
		t.Errorf("scopes = %q, want empty", id.Scopes)
	}
}
