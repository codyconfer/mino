package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/errs"
)

func TestGLabHostname(t *testing.T) {
	cases := []struct {
		apiURL string
		want   string
	}{
		{"", ""},
		{"https://gitlab.com/api/v4", "gitlab.com"},
		{"https://gitlab.example.com/api/v4", "gitlab.example.com"},
		{"https://GITLAB.Example.COM/api/v4", "gitlab.example.com"},
		{"https://gitlab.example.com:8443/api/v4", "gitlab.example.com:8443"},
		{"://bad", ""},
	}
	for _, c := range cases {
		if got := GLabHostname(c.apiURL); got != c.want {
			t.Errorf("GLabHostname(%q) = %q, want %q", c.apiURL, got, c.want)
		}
	}
}

func TestGitLabAPIBaseAppendsTheRESTPath(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "https://gitlab.com/api/v4"},
		{"   ", "https://gitlab.com/api/v4"},
		{"https://gitlab.com", "https://gitlab.com/api/v4"},
		{"https://gitlab.example.com/", "https://gitlab.example.com/api/v4"},
		{"https://gitlab.example.com/api/v4", "https://gitlab.example.com/api/v4"},
		{"https://gitlab.example.com/api/v4/", "https://gitlab.example.com/api/v4"},
		{"https://gitlab.example.com/gitlab", "https://gitlab.example.com/gitlab"},
	}
	for _, c := range cases {
		if got := GitLabAPIBase(c.in); got != c.want {
			t.Errorf("GitLabAPIBase(%q) = %q, want %q; GitLab users type the instance root where "+
				"GitHub users type the API root", c.in, got, c.want)
		}
	}
}

func TestGitLabInstanceURLStripsTheAPIPath(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "https://gitlab.com"},
		{"https://gitlab.com/api/v4", "https://gitlab.com"},
		{"https://gitlab.example.com/api/v4", "https://gitlab.example.com"},
		{"https://gitlab.example.com:8443/api/v4", "https://gitlab.example.com:8443"},
	}
	for _, c := range cases {
		if got := GitLabInstanceURL(c.in); got != c.want {
			t.Errorf("GitLabInstanceURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func fakeGLab(t *testing.T, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake glab shell script requires a POSIX shell")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "glab"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("GITLAB_TOKEN", "")
	t.Setenv("GL_TOKEN", "")
}

func mustParseTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

func clearAmbientGitLab(t *testing.T) {
	t.Helper()
	t.Setenv("GITLAB_TOKEN", "")
	t.Setenv("GL_TOKEN", "")
}

func withoutGLabOnPath(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
}

func staticGitLabSelectionFor(base string) GitLabSelection {
	return GitLabSelection{
		Mech:   GitLabEnvToken,
		Origin: originGitLabToken,
		APIURL: base,
		src:    staticSource{token: "glpat-secret"},
	}
}

func TestGLAPIGetSendsBearerAndNeverShellsOutToGLab(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	fakeGLab(t, "#!/bin/sh\necho \"$@\" > \""+argsFile+"\"\n")

	var gotAuth, gotURI string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotURI = r.Header.Get("Authorization"), r.RequestURI
		w.Write([]byte(`{"username":"cody"}`))
	}))
	defer srv.Close()

	if _, err := GLAPIGet(context.Background(), staticGitLabSelectionFor(srv.URL+"/api/v4"), "user"); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer glpat-secret" {
		t.Errorf("Authorization = %q, want a bearer token; GitLab accepts bearer for PATs, group "+
			"and project tokens and OAuth alike, so one header covers the whole ladder", gotAuth)
	}
	if gotURI != "/api/v4/user" {
		t.Errorf("request URI = %q, want /api/v4/user", gotURI)
	}
	if _, err := os.Stat(argsFile); err == nil {
		t.Error("the provider shelled out to glab; it must use HTTP so RateLimit can read response " +
			"headers, which a CLI does not hand back")
	}
}

func TestGLAPIGetRequestsAFullPageOnListEndpoints(t *testing.T) {
	clearAmbientGitLab(t)
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.RequestURI)
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	sel := staticGitLabSelectionFor(srv.URL + "/api/v4")
	for _, path := range []string{"user/keys", "user/gpg_keys", "user/emails", "user"} {
		if _, err := GLAPIGet(context.Background(), sel, path); err != nil {
			t.Fatal(err)
		}
	}
	for i, path := range []string{"user/keys", "user/gpg_keys", "user/emails"} {
		if !strings.Contains(seen[i], "per_page=100") {
			t.Errorf("%s requested as %q with no per_page; GitLab defaults these to 20 rows, so a "+
				"user with more keys than that is told a registered key is missing", path, seen[i])
		}
	}
	if strings.Contains(seen[3], "per_page") {
		t.Errorf("/user is not a list endpoint but got %q", seen[3])
	}
}

func TestClassifyGitLabStatus(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		header   http.Header
		body     string
		wantKind errs.Kind
		wantHint string
	}{
		{
			name: "unauthorized", status: http.StatusUnauthorized,
			body:     `{"message":"401 Unauthorized"}`,
			wantKind: errs.KindAuth, wantHint: "glab auth login",
		},
		{
			name: "insufficient scope", status: http.StatusForbidden,
			body:     `{"error":"insufficient_scope","scope":"read_api"}`,
			wantKind: errs.KindAuth, wantHint: "read_api",
		},
		{
			name: "rate limited by header", status: http.StatusForbidden,
			header:   http.Header{"Ratelimit-Remaining": {"0"}},
			wantKind: errs.KindSignal, wantHint: "rate limit",
		},
		{
			name: "too many requests", status: http.StatusTooManyRequests,
			header:   http.Header{"Retry-After": {"120"}},
			wantKind: errs.KindSignal, wantHint: "retry after 2m0s",
		},
		{
			name: "plain forbidden", status: http.StatusForbidden,
			body:     `{"message":"403 Forbidden"}`,
			wantKind: errs.KindAuth, wantHint: "glab auth login",
		},
		{
			name: "server error", status: http.StatusInternalServerError,
			wantKind: errs.KindSignal,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: c.status, Status: http.StatusText(c.status), Header: c.header}
			if resp.Header == nil {
				resp.Header = http.Header{}
			}
			err := classifyGitLabStatus(resp, c.body)
			if errs.KindOf(err) != c.wantKind {
				t.Errorf("kind = %v, want %v (%v)", errs.KindOf(err), c.wantKind, err)
			}
			if c.wantHint != "" && !strings.Contains(errs.Hint(err), c.wantHint) {
				t.Errorf("hint = %q, want it to mention %q", errs.Hint(err), c.wantHint)
			}
		})
	}
}

func TestGitLabRetryAfterReadsUnprefixedHeaders(t *testing.T) {
	now := mustParseTime(t, "2026-08-05T12:00:00Z")
	cases := []struct {
		name    string
		header  http.Header
		wantSec float64
		wantOK  bool
	}{
		{"no headers", http.Header{}, 0, false},
		{"retry-after seconds", http.Header{"Retry-After": {"90"}}, 90, true},
		{
			"ratelimit reset epoch",
			http.Header{"Ratelimit-Remaining": {"0"}, "Ratelimit-Reset": {"1785931260"}},
			60, true,
		},
		{
			"reset in the past",
			http.Header{"Ratelimit-Remaining": {"0"}, "Ratelimit-Reset": {"1"}},
			0, false,
		},
		{
			"remaining above zero is not a limit",
			http.Header{"Ratelimit-Remaining": {"12"}, "Ratelimit-Reset": {"1785931260"}},
			0, false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, ok := gitlabRetryAfter(c.header, now)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if ok && d.Seconds() != c.wantSec {
				t.Errorf("delay = %v, want %vs", d, c.wantSec)
			}
		})
	}
}

func TestGitLabRetryAfterIsBounded(t *testing.T) {
	now := mustParseTime(t, "2026-08-05T12:00:00Z")
	d, ok := gitlabRetryAfter(http.Header{"Retry-After": {"999999"}}, now)
	if !ok || d != maxGitLabRetryAfter {
		t.Errorf("delay = %v/%v, want it clamped to %v so a bad header cannot stall the poller "+
			"indefinitely", d, ok, maxGitLabRetryAfter)
	}
}
