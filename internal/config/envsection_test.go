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
