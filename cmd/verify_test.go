package cmd

import (
	"strings"
	"testing"

	"github.com/codyconfer/sisyphus/redact"
)

func TestMaskSecrets(t *testing.T) {
	in := strings.Join([]string{
		"oauth_client_id: Iv1.abc123",
		"oauth_client_secret: super-secret-value",
		"token_env: SLACK_TOKEN",
		"access_token: ya29.leaky",
		"query: is:open is:pr",
	}, "\n")
	got := redact.Line(in)

	if strings.Contains(got, "super-secret-value") {
		t.Error("client secret was not masked")
	}
	if strings.Contains(got, "ya29.leaky") {
		t.Error("access_token was not masked")
	}

	if !strings.Contains(got, "Iv1.abc123") {
		t.Error("client_id (not a secret) should be preserved")
	}
	if !strings.Contains(got, "SLACK_TOKEN") {
		t.Error("token_env is a var name, not a secret; should be preserved")
	}
	if !strings.Contains(got, "is:open is:pr") {
		t.Error("query value should be preserved")
	}
}

func TestSecretKey(t *testing.T) {
	for _, k := range []string{"oauth_client_secret", "password", "access_token", "refresh_token"} {
		if !redact.Key(k) {
			t.Errorf("%q should be treated as secret", k)
		}
	}
	for _, k := range []string{"oauth_client_id", "token_env", "query", "name"} {
		if redact.Key(k) {
			t.Errorf("%q should NOT be treated as secret", k)
		}
	}
}
