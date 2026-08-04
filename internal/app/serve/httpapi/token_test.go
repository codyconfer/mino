package httpapi

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
)

func TestResolveTokenGeneratesAndPersists(t *testing.T) {
	home := t.TempDir()
	tok, source, err := ResolveToken(home, "")
	if err != nil {
		t.Fatalf("ResolveToken: %v", err)
	}
	if len(tok) < 40 {
		t.Errorf("generated token is only %d characters; it is the only thing guarding the trigger API", len(tok))
	}
	path := config.HTTPTokenPath(home)
	if source != path {
		t.Errorf("source = %q, want the token path %q so the startup line can point at it", source, path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the token was not persisted: %v", err)
	}
	if strings.TrimSpace(string(b)) != tok {
		t.Errorf("persisted token %q does not match the returned one %q", strings.TrimSpace(string(b)), tok)
	}
}

func TestGeneratedTokenFileIsOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix file modes do not apply on windows")
	}
	home := t.TempDir()
	if _, _, err := ResolveToken(home, ""); err != nil {
		t.Fatalf("ResolveToken: %v", err)
	}
	fi, err := os.Stat(config.HTTPTokenPath(home))
	if err != nil {
		t.Fatal(err)
	}
	// Anything group- or world-readable hands every local account the ability to
	// trigger flights and actions.
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("token file mode = %04o, want 0600", perm)
	}
}

func TestResolveTokenReusesAnExistingFile(t *testing.T) {
	home := t.TempDir()
	first, _, err := ResolveToken(home, "")
	if err != nil {
		t.Fatalf("ResolveToken: %v", err)
	}
	second, _, err := ResolveToken(home, "")
	if err != nil {
		t.Fatalf("ResolveToken: %v", err)
	}
	// Regenerating on every restart would invalidate every script the user wrote.
	if first != second {
		t.Errorf("token changed across restarts: %q then %q", first, second)
	}
}

func TestConfiguredTokenWinsAndIsNotPersisted(t *testing.T) {
	home := t.TempDir()
	const configured = "a-configured-token-value"
	tok, source, err := ResolveToken(home, configured)
	if err != nil {
		t.Fatalf("ResolveToken: %v", err)
	}
	if tok != configured {
		t.Errorf("token = %q, want the configured %q", tok, configured)
	}
	if source != "daemon.http.token" {
		t.Errorf("source = %q, want daemon.http.token", source)
	}
	if _, err := os.Stat(config.HTTPTokenPath(home)); err == nil {
		t.Error("a configured token was also written to disk, spreading the secret for no reason")
	}
}

func TestShortConfiguredTokenIsRejected(t *testing.T) {
	_, _, err := ResolveToken(t.TempDir(), "short")
	if err == nil {
		t.Fatal("a 5-character token was accepted; it is the only guard on the trigger API")
	}
	if errs.KindOf(err) != errs.KindConfig {
		t.Errorf("kind = %q, want config", errs.KindOf(err))
	}
	if errs.Hint(err) == "" {
		t.Error("no hint telling the user to leave it unset and let mino generate one")
	}
}

func TestResolveTokenTrimsWhitespace(t *testing.T) {
	home := t.TempDir()
	// A hand-edited or `echo`-written file almost always ends in a newline.
	if _, _, err := ResolveToken(home, ""); err != nil {
		t.Fatalf("ResolveToken: %v", err)
	}
	tok, _, err := ResolveToken(home, "")
	if err != nil {
		t.Fatalf("ResolveToken: %v", err)
	}
	if strings.ContainsAny(tok, " \t\r\n") {
		t.Errorf("token %q carries whitespace, which would never match a Bearer header", tok)
	}
}
