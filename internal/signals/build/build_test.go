package build

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
	gh "github.com/codyconfer/mino/internal/signals/github"
)

func fakeGHOnPath(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake gh shell script requires a POSIX shell")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

func TestGithubBackendPinsConfiguredHostname(t *testing.T) {
	fakeGHOnPath(t)

	cfg := &config.Config{}
	cfg.GitHub.APIURL = "https://ghe.example.com/api/v3"
	backend, err := githubBackend(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	cli, ok := backend.(gh.CLIBackend)
	if !ok {
		t.Fatalf("backend = %T, want gh.CLIBackend", backend)
	}
	if cli.Hostname != "ghe.example.com" {
		t.Fatalf("Hostname = %q, want ghe.example.com", cli.Hostname)
	}

	cfg.GitHub.APIURL = ""
	backend, err = githubBackend(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	cli, ok = backend.(gh.CLIBackend)
	if !ok {
		t.Fatalf("backend = %T, want gh.CLIBackend", backend)
	}
	if cli.Hostname != "" {
		t.Fatalf("Hostname = %q, want empty", cli.Hostname)
	}
}

func TestResolveWriteTarget(t *testing.T) {
	if _, err := ResolveWriteTarget("task list", "tasks.list", "", ""); err == nil {
		t.Error("expected error when no writable target is configured")
	}

	if got, err := ResolveWriteTarget("task list", "tasks.list", "My Tasks", ""); err != nil || got != "My Tasks" {
		t.Errorf("default target = %q, %v", got, err)
	}

	if got, err := ResolveWriteTarget("task list", "tasks.list", "My Tasks", "my tasks"); err != nil || got != "My Tasks" {
		t.Errorf("matching target = %q, %v", got, err)
	}

	if _, err := ResolveWriteTarget("directory", "drive.dir", "Inbox", "Someone Else's Folder"); err == nil {
		t.Error("expected other targets to be rejected as read-only")
	}
}

func TestGithubBackendPrefersServiceAuthOverTheGHCLI(t *testing.T) {
	fakeGHOnPath(t)

	cfg := &config.Config{}
	cfg.GitHub.APIURL = "https://ghe.example.com/api/v3"
	cfg.GitHub.ServiceToken = "ghp_service"

	backend, err := githubBackend(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	api, ok := backend.(gh.APIBackend)
	if !ok {
		t.Fatalf("backend = %T, want gh.APIBackend; a gh CLI on PATH must not outrank an explicitly "+
			"configured service identity, or mino acts as whoever ran `gh auth login`", backend)
	}
	if api.BaseURL != "https://ghe.example.com/api/v3" {
		t.Errorf("BaseURL = %q, want the configured api_url", api.BaseURL)
	}
	tok, err := api.Auth.Token(context.Background())
	if err != nil || tok != "ghp_service" {
		t.Errorf("Token() = %q, %v; want the service token", tok, err)
	}
}

func TestGithubBackendRefusesAHalfConfiguredApp(t *testing.T) {
	fakeGHOnPath(t)
	t.Setenv("GITHUB_TOKEN", "a-human-token")

	cfg := &config.Config{}
	cfg.GitHub.App.ID = "123456"

	if _, err := githubBackend(cfg, nil); err == nil {
		t.Fatal("githubBackend accepted a half-configured app and fell back to another identity; every " +
			"flight, action and comment would then run as that person")
	}
}

func TestBuildActiveGithubExplainsMissingAuthInsteadOfErrNoActive(t *testing.T) {
	withoutGH(t)

	_, err := buildActiveGithub(nil, &config.Config{}, nil, nil)
	if err == nil {
		t.Fatal("buildActiveGithub succeeded with no authentication")
	}
	if errors.Is(err, ErrNoActive) {
		t.Fatal("returned ErrNoActive for an auth problem; serve logs that at debug and then reports the " +
			"generic \"no signals with realtime or scheduled support\", so the real cause is invisible")
	}
	if errs.KindOf(err) != errs.KindAuth {
		t.Errorf("kind = %v, want KindAuth", errs.KindOf(err))
	}
}

func TestBuildActiveGithubRefusesAppOnlyAuthWithAReason(t *testing.T) {
	withoutGH(t)
	key := writeTestAppKey(t)

	cfg := &config.Config{}
	cfg.GitHub.App.ID = "123456"
	cfg.GitHub.App.InstallationID = "78901234"
	cfg.GitHub.App.PrivateKeyPath = key

	_, err := buildActiveGithub(nil, cfg, nil, nil)
	if err == nil {
		t.Fatal("buildActiveGithub accepted App-only auth; /notifications is scoped to the authenticated " +
			"user, so an installation token gets 403 and the stream would fail every poll instead of at build")
	}
	if errors.Is(err, ErrNoActive) {
		t.Fatal("returned ErrNoActive, which serve logs at debug only")
	}
	msg := err.Error() + " " + errs.Hint(err)
	for _, want := range []string{"notifications", "github.service_token"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message does not mention %q, so the operator has no way to know the fix: %s", want, msg)
		}
	}
}

func withoutGH(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("MINO_GITHUB_APP_PRIVATE_KEY", "")
}

func writeTestAppKey(t *testing.T) string {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "app.pem")
	data := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestServiceAuthWarnsAboutAtMeQueriesWhenNoViewerIsSet(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	withoutGH(t)

	cfg := &config.Config{}
	cfg.GitHub.ServiceToken = "ghp_service"

	if _, err := buildGithub(nil, cfg, nil, nil); err != nil {
		t.Fatalf("buildGithub: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "@me") || !strings.Contains(out, "github.viewer") {
		t.Errorf("no usable warning for @me under a service identity:\n%s\nThe stock queries are "+
			"author:@me and review-requested:@me, so a correctly configured service account returns "+
			"empty results and looks broken with nothing in the log to explain it", out)
	}
}

func TestAViewerSilencesTheAtMeWarning(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	withoutGH(t)

	cfg := &config.Config{}
	cfg.GitHub.ServiceToken = "ghp_service"
	cfg.GitHub.Viewer = "octocat"

	if _, err := buildGithub(nil, cfg, nil, nil); err != nil {
		t.Fatalf("buildGithub: %v", err)
	}
	if strings.Contains(buf.String(), "@me") {
		t.Errorf("warned about @me even though github.viewer resolves it:\n%s", buf.String())
	}
}

func TestAHumanTokenIsNeverWarnedAboutAtMe(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	withoutGH(t)
	t.Setenv("GITHUB_TOKEN", "a-human-token")

	cfg := &config.Config{}
	if _, err := buildGithub(nil, cfg, nil, nil); err != nil {
		t.Fatalf("buildGithub: %v", err)
	}
	if strings.Contains(buf.String(), "@me") {
		t.Errorf("warned a human about @me, which is exactly the qualifier they want:\n%s", buf.String())
	}
}
