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
)

func TestGHHostname(t *testing.T) {
	cases := []struct {
		apiURL string
		want   string
	}{
		{"", ""},
		{"https://api.github.com", "github.com"},
		{"https://ghe.example.com/api/v3", "ghe.example.com"},
		{"https://GHE.Example.COM/api/v3", "ghe.example.com"},
		{"https://ghe.example.com:8443/api/v3", "ghe.example.com:8443"},
		{"://bad", ""},
	}
	for _, c := range cases {
		if got := GHHostname(c.apiURL); got != c.want {
			t.Errorf("GHHostname(%q) = %q, want %q", c.apiURL, got, c.want)
		}
	}
}

func fakeGH(t *testing.T, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake gh shell script requires a POSIX shell")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
}

func TestGHAPIGetPinsConfiguredHostname(t *testing.T) {
	fakeGH(t, "#!/bin/sh\necho \"$@\"\n")

	out, err := GHAPIGet(context.Background(), ambientSelection(t, "https://ghe.example.com/api/v3"), "user")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "api --hostname ghe.example.com user\n" {
		t.Fatalf("gh args = %q", out)
	}

	out, err = GHAPIGet(context.Background(), ambientSelection(t, ""), "user")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "api user\n" {
		t.Fatalf("gh args = %q", out)
	}
}

func TestGitHubAuthStatusPinsConfiguredHostname(t *testing.T) {
	fakeGH(t, "#!/bin/sh\necho \"$@\" > \"$MINO_TEST_GH_ARGS\"\n")
	argsFile := filepath.Join(t.TempDir(), "args")
	t.Setenv("MINO_TEST_GH_ARGS", argsFile)

	ok, detail := GitHubAuthStatus(context.Background(), ambientSelection(t, "https://ghe.example.com/api/v3"))
	if !ok || detail != "gh CLI is logged in" {
		t.Fatalf("GitHubAuthStatus = %v, %q", ok, detail)
	}
	recorded, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(recorded)); got != "auth status --hostname ghe.example.com" {
		t.Fatalf("gh args = %q", got)
	}
}

func TestGitHubAuthStatusUnpinnedWhenUnset(t *testing.T) {
	fakeGH(t, "#!/bin/sh\necho \"$@\" > \"$MINO_TEST_GH_ARGS\"\n")
	argsFile := filepath.Join(t.TempDir(), "args")
	t.Setenv("MINO_TEST_GH_ARGS", argsFile)

	ok, detail := GitHubAuthStatus(context.Background(), ambientSelection(t, ""))
	if !ok || detail != "gh CLI is logged in" {
		t.Fatalf("GitHubAuthStatus = %v, %q", ok, detail)
	}
	recorded, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(recorded)); got != "auth status" {
		t.Fatalf("gh args = %q", got)
	}
}

func TestGHAPIGetUsesServiceAuthAndNeverShellsOutToGH(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"login":"machine-user"}`))
	}))
	defer srv.Close()

	argsFile := filepath.Join(t.TempDir(), "gh-args")
	fakeGH(t, "#!/bin/sh\necho \"$@\" > "+argsFile+"\n")

	sel, err := SelectGitHub(GitHubSpec{APIURL: srv.URL, ServiceToken: "ghp_service", Store: memStore{}})
	if err != nil {
		t.Fatalf("SelectGitHub: %v", err)
	}
	if _, err := GHAPIGet(context.Background(), sel, "user"); err != nil {
		t.Fatalf("GHAPIGet: %v", err)
	}

	if gotAuth != "Bearer ghp_service" {
		t.Errorf("Authorization = %q, want the service token", gotAuth)
	}
	if _, err := os.Stat(argsFile); err == nil {
		t.Error("GHAPIGet shelled out to `gh api` while a service identity was configured; gh would " +
			"authenticate as whoever ran `gh auth login`, so the call would silently return that " +
			"person's data instead of the service's")
	}
}

func TestGHAPIGetStillUsesTheGHCLIWhenNoServiceAuth(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "gh-args")
	fakeGH(t, "#!/bin/sh\necho \"$@\" > "+argsFile+"\necho '{}'\n")

	sel, err := SelectGitHub(GitHubSpec{Store: memStore{}})
	if err != nil {
		t.Fatalf("SelectGitHub: %v", err)
	}
	if !sel.UsesGHCLI() {
		t.Fatalf("mech = %v, want the gh CLI when nothing else is configured", sel.Mech)
	}
	if _, err := GHAPIGet(context.Background(), sel, "user"); err != nil {
		t.Fatalf("GHAPIGet: %v", err)
	}
	if _, err := os.Stat(argsFile); err != nil {
		t.Error("GHAPIGet did not use `gh api` despite the CLI being the selected mechanism")
	}
}
