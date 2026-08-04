package config

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/log"
)

func TestParseConfigWarnsOnAnEnvVarNamingASection(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	t.Setenv("MINO_GITHUB", "acme")

	if _, err := ParseConfig(t.TempDir(), []byte("output: json\n"), "yaml"); err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "MINO_GITHUB") {
		t.Fatalf("MINO_GITHUB was dropped without a word; a plausible-looking env var that sets nothing is "+
			"indistinguishable from one that works:\n%s", out)
	}
	if !strings.Contains(out, "github") {
		t.Errorf("warning does not name the section it collides with:\n%s", out)
	}
}

func TestParseConfigDoesNotWarnForALeafEnvVar(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	t.Setenv("MINO_OUTPUT", "json")

	cfg, err := ParseConfig(t.TempDir(), nil, "yaml")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Output != "json" {
		t.Fatalf("MINO_OUTPUT did not apply: output = %q", cfg.Output)
	}
	if strings.Contains(buf.String(), "MINO_OUTPUT") {
		t.Errorf("warned about a leaf env var that applied correctly:\n%s", buf.String())
	}
}

func TestParseConfigAppliesThreeLevelHTTPEnvVars(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	t.Setenv("MINO_DAEMON_HTTP_ENABLED", "true")
	t.Setenv("MINO_DAEMON_HTTP_PORT", "9999")
	t.Setenv("MINO_DAEMON_HTTP_TOKEN", "sekret")

	cfg, err := ParseConfig(t.TempDir(), nil, "yaml")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	// daemon.http is the deepest config nesting mino has; if the env overlay
	// stops at two levels these silently do nothing and the API looks
	// unconfigurable without a config file.
	if !cfg.Daemon.HTTP.Enabled {
		t.Errorf("MINO_DAEMON_HTTP_ENABLED did not apply: enabled = false")
	}
	if cfg.Daemon.HTTP.Port != 9999 {
		t.Errorf("MINO_DAEMON_HTTP_PORT did not apply: port = %d, want 9999", cfg.Daemon.HTTP.Port)
	}
	if cfg.Daemon.HTTP.Token != "sekret" {
		t.Errorf("MINO_DAEMON_HTTP_TOKEN did not apply: token = %q", cfg.Daemon.HTTP.Token)
	}
	if strings.Contains(buf.String(), "MINO_DAEMON_HTTP") {
		t.Errorf("warned about env vars that applied correctly:\n%s", buf.String())
	}
}
