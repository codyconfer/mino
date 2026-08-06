package build

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/plugin"
	gl "github.com/codyconfer/mino/internal/signals/gitlab"
)

func fakeGLabOnPath(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake glab shell script requires a POSIX shell")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "glab"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("GITLAB_TOKEN", "")
	t.Setenv("GL_TOKEN", "")
}

func withoutGLab(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
	t.Setenv("GITLAB_TOKEN", "")
	t.Setenv("GL_TOKEN", "")
}

func TestGitlabIsARegisteredStockSignal(t *testing.T) {
	if !HasBuilder("gitlab") {
		t.Fatal("gitlab has no query builder; `mino gitlab query` would report an unknown signal")
	}
	if !HasActiveBuilder("gitlab") {
		t.Fatal("gitlab has no stream builder, but the descriptor advertises CapStream")
	}
	for _, cap := range []plugin.Capability{
		plugin.CapQuery, plugin.CapStream, plugin.CapCacheable, plugin.CapDetail,
	} {
		if !plugin.HasCapability("gitlab", cap) {
			t.Errorf("gitlab does not advertise %v", cap)
		}
	}
	if !contains(DetailSignals(), "gitlab") {
		t.Errorf("DetailSignals = %v, want gitlab; `mino gitlab show` is added from that list",
			DetailSignals())
	}
}

func TestStockBuildersAreGuardedPerSignal(t *testing.T) {
	if _, ok := plugin.LookupBuilders("github"); !ok {
		t.Fatal("github lost its builders")
	}
	if _, ok := plugin.LookupBuilders("gitlab"); !ok {
		t.Fatal("gitlab lost its builders; an all-or-nothing guard would let an external override of " +
			"one stock signal suppress the other")
	}
}

func TestGitlabBackendPinsConfiguredHostname(t *testing.T) {
	fakeGLabOnPath(t)

	cfg := config.Defaults()
	cfg.GitLab.APIURL = "https://gitlab.example.com"
	sig, err := buildGitlab(nil, cfg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if sig == nil {
		t.Fatal("buildGitlab returned no signal")
	}

	sel, err := gitlabAuth(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	backend, _, err := gitlabBackendFor(sel)
	if err != nil {
		t.Fatal(err)
	}
	cli, ok := backend.(gl.CLIBackend)
	if !ok {
		t.Fatalf("backend = %T, want gl.CLIBackend", backend)
	}
	if cli.Hostname != "gitlab.example.com" {
		t.Errorf("Hostname = %q, want gitlab.example.com", cli.Hostname)
	}
}

func TestGitlabServiceAuthOutranksTheCLI(t *testing.T) {
	fakeGLabOnPath(t)

	cfg := config.Defaults()
	cfg.GitLab.ServiceToken = "glpat-service"
	sel, err := gitlabAuth(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !sel.ServiceIdentity() {
		t.Fatalf("mech = %v with a service token configured", sel.Mech)
	}
	backend, rate, err := gitlabBackendFor(sel)
	if err != nil {
		t.Fatal(err)
	}
	api, ok := backend.(gl.APIBackend)
	if !ok {
		t.Fatalf("backend = %T, want gl.APIBackend", backend)
	}
	if api.BaseURL != "https://gitlab.com/api/v4" {
		t.Errorf("BaseURL = %q, want the gitlab.com REST base when api_url is unset", api.BaseURL)
	}
	if rate == nil {
		t.Error("the REST backend got no rate hint, so the stream cannot honour Retry-After")
	}
}

func TestBuildGitlabRejectsABadAPIURL(t *testing.T) {
	cfg := config.Defaults()
	cfg.GitLab.APIURL = "http://gitlab.example.com"
	_, err := buildGitlab(nil, cfg, nil, nil)
	if err == nil {
		t.Fatal("a plain-http api_url built a signal")
	}
	if errs.KindOf(err) != errs.KindConfig {
		t.Errorf("kind = %v, want config", errs.KindOf(err))
	}
}

func TestBuildGitlabRejectsABadSelector(t *testing.T) {
	fakeGLabOnPath(t)

	cfg := config.Defaults()
	_, err := buildGitlab(map[string]string{"query": "kind:mr nonesuch:x"}, cfg, nil, nil)
	if err == nil {
		t.Fatal("a bad selector built a signal")
	}
	if errs.KindOf(err) != errs.KindConfig {
		t.Errorf("kind = %v, want config", errs.KindOf(err))
	}
}

func TestBuildActiveGitlabExplainsMissingAuth(t *testing.T) {
	withoutGLab(t)

	cfg := config.Defaults()
	_, err := buildActiveGitlab(nil, cfg, nil, nil)
	if err == nil {
		t.Fatal("realtime built with no GitLab authentication")
	}
	if err == ErrNoActive {
		t.Fatal("realtime reported \"no active implementation\" when the real problem is that nothing " +
			"is authenticated")
	}
	if errs.KindOf(err) != errs.KindAuth {
		t.Errorf("kind = %v, want auth", errs.KindOf(err))
	}
	for _, want := range []string{"gitlab.service_token", "glab auth login", "GITLAB_TOKEN", "mino login gitlab"} {
		if !strings.Contains(errs.Hint(err), want) {
			t.Errorf("hint = %q, want it to mention %q", errs.Hint(err), want)
		}
	}
}

func TestBuildActiveGitlabAcceptsAServiceToken(t *testing.T) {
	withoutGLab(t)

	cfg := config.Defaults()
	cfg.GitLab.ServiceToken = "glpat-service"
	src, err := buildActiveGitlab(nil, cfg, nil, nil)
	if err != nil {
		t.Fatalf("a service token was refused for realtime: %v; GitLab has no /notifications "+
			"asymmetry, so the refusal buildActiveGithub makes for a GitHub App does not apply", err)
	}
	if src == nil {
		t.Fatal("no stream was built")
	}
}

func TestBuildActiveGitlabHonoursTheIntervalParam(t *testing.T) {
	withoutGLab(t)

	cfg := config.Defaults()
	cfg.GitLab.ServiceToken = "glpat-service"
	src, err := buildActiveGitlab(map[string]string{"interval": "30s"}, cfg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := src.LatencyFloor().Seconds(); got != 30 {
		t.Errorf("LatencyFloor = %vs, want the configured 30s", got)
	}

	if _, err := buildActiveGitlab(map[string]string{"interval": "nope"}, cfg, nil, nil); err == nil {
		t.Error("an unparseable interval was accepted")
	}
}

func TestServiceIdentityWithoutAViewerWarnsRatherThanFails(t *testing.T) {
	withoutGLab(t)

	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	cfg := config.Defaults()
	cfg.GitLab.ServiceToken = "glpat-service"
	if _, err := buildGitlab(map[string]string{"query": "kind:mr reviewer:@me"}, cfg, nil, nil); err != nil {
		t.Fatalf("a viewerless @me failed the build: %v; /user does resolve for a bot token, it just "+
			"resolves to the wrong person, so a warning beats an error", err)
	}
	if !strings.Contains(buf.String(), "gitlab.viewer") {
		t.Errorf("log = %q, want a warning naming gitlab.viewer", buf.String())
	}
}

func TestConfiguredViewerSuppressesTheWarning(t *testing.T) {
	withoutGLab(t)

	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	cfg := config.Defaults()
	cfg.GitLab.ServiceToken = "glpat-service"
	cfg.GitLab.Viewer = "acme-bot"
	if _, err := buildGitlab(map[string]string{"query": "kind:mr reviewer:@me"}, cfg, nil, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "gitlab.viewer") {
		t.Errorf("log = %q, want no warning once gitlab.viewer is set", buf.String())
	}
}

func TestGitlabQueryParamsAreRegistered(t *testing.T) {
	keys := ParamKeys("gitlab")
	for _, want := range []string{"query", "title", "interval"} {
		if !contains(keys, want) {
			t.Errorf("gitlab params = %v, want %q", keys, want)
		}
	}
	for _, spec := range QueryParams("gitlab") {
		if spec.Key != "query" {
			continue
		}
		if spec.Delim != " " || len(spec.Values) == 0 {
			t.Errorf("query param = %+v, want space-delimited completion values so the query builder "+
				"and shell completion can offer the vocabulary", spec)
		}
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
