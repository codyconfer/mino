package github

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/signals"
)

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
}

func TestCLIBackendPinsConfiguredHostname(t *testing.T) {
	fakeGH(t, "#!/bin/sh\necho \"$@\"\n")

	b := CLIBackend{Hostname: "ghe.example.com"}
	out, err := b.SearchIssues(context.Background(), "is:open", 5)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "api --hostname ghe.example.com -X GET search/issues -f q=is:open -f per_page=5\n" {
		t.Fatalf("gh args = %q", out)
	}

	out, err = b.GraphQL(context.Background(), "query{viewer{login}}", map[string]any{"first": 5, "login": "octo"})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "api --hostname ghe.example.com graphql -f query=query{viewer{login}} -F first=5 -f login=octo\n" {
		t.Fatalf("gh args = %q", out)
	}

	out, err = b.WorkflowRuns(context.Background(), "codyconfer", "mino", 1)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "api --hostname ghe.example.com -X GET repos/codyconfer/mino/actions/runs -f per_page=1\n" {
		t.Fatalf("gh args = %q", out)
	}

	out, err = b.WorkflowJobs(context.Background(), "codyconfer", "mino", 30706047121)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "api --hostname ghe.example.com -X GET repos/codyconfer/mino/actions/runs/30706047121/jobs -f per_page=100\n" {
		t.Fatalf("gh args = %q", out)
	}
}

func TestCLIBackendUnpinnedWhenHostnameUnset(t *testing.T) {
	fakeGH(t, "#!/bin/sh\necho \"$@\"\n")

	b := CLIBackend{}
	out, err := b.SearchIssues(context.Background(), "is:open", 5)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "api -X GET search/issues -f q=is:open -f per_page=5\n" {
		t.Fatalf("gh args = %q", out)
	}

	out, err = b.GraphQL(context.Background(), "query{viewer{login}}", nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "api graphql -f query=query{viewer{login}}\n" {
		t.Fatalf("gh args = %q", out)
	}
}

func TestAPIBackendSearch(t *testing.T) {
	var gotAuth, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"title":"fix","html_url":"https://x/1","repository_url":"https://api.github.com/repos/o/r","user":{"login":"alice"}}]}`))
	}))
	defer srv.Close()

	b := APIBackend{Token: "tok123", BaseURL: srv.URL, HTTP: srv.Client()}
	raw, err := b.SearchIssues(context.Background(), "is:open is:pr", 10)
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("auth header = %q, want Bearer tok123", gotAuth)
	}
	if gotQuery != "is:open is:pr" {
		t.Errorf("query = %q", gotQuery)
	}

	sec, err := mapSearchResponse(raw, "PRs")
	if err != nil {
		t.Fatal(err)
	}
	if len(sec.Items) != 1 || sec.Items[0].Title != "fix" || sec.Items[0].Subtitle != "o/r" {
		t.Fatalf("mapped section wrong: %#v", sec.Items)
	}
}

func TestAPIBackendErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
	}))
	defer srv.Close()

	b := APIBackend{Token: "bad", BaseURL: srv.URL, HTTP: srv.Client()}
	_, err := b.SearchIssues(context.Background(), "q", 10)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401 error, got %v", err)
	}
}

func TestAPIBackendActions(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	b := APIBackend{Token: "tok", BaseURL: srv.URL, HTTP: srv.Client()}
	if _, err := b.WorkflowRuns(context.Background(), "codyconfer", "mino", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := b.WorkflowJobs(context.Background(), "codyconfer", "mino", 30706047121); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/repos/codyconfer/mino/actions/runs?per_page=1",
		"/repos/codyconfer/mino/actions/runs/30706047121/jobs?per_page=100",
	}
	if len(paths) != len(want) || paths[0] != want[0] || paths[1] != want[1] {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
}

func endlessBodyServer(t *testing.T, mib int) *httptest.Server {
	t.Helper()
	chunk := bytes.Repeat([]byte("a"), 64<<10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		for range mib * 16 {
			if _, err := w.Write(chunk); err != nil {
				return
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			if r.Context().Err() != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAPIBackendSearchBoundsBody(t *testing.T) {
	srv := endlessBodyServer(t, 12)
	b := APIBackend{Token: "tok", BaseURL: srv.URL, HTTP: srv.Client()}
	body, err := b.SearchIssues(context.Background(), "q", 10)
	if err == nil {
		t.Fatalf("read %d bytes with no error: the response body is unbounded", len(body))
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error = %v, want a size-limit error", err)
	}
}

func TestAPIBackendGraphQLBoundsBody(t *testing.T) {
	srv := endlessBodyServer(t, 12)
	b := APIBackend{Token: "tok", BaseURL: srv.URL, HTTP: srv.Client()}
	body, err := b.GraphQL(context.Background(), "query{viewer{login}}", nil)
	if err == nil {
		t.Fatalf("read %d bytes with no error: the response body is unbounded", len(body))
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error = %v, want a size-limit error", err)
	}
}

func TestAPIBackendFallsBackToSharedClient(t *testing.T) {
	if (APIBackend{}).client() != signals.HTTPClient() {
		t.Error("APIBackend must fall back to the shared client, not http.DefaultClient")
	}
	if (APIBackend{}).client().Timeout <= 0 {
		t.Error("the fallback client has no request timeout")
	}
	own := &http.Client{}
	if (APIBackend{HTTP: own}).client() != own {
		t.Error("an injected client must be preserved")
	}
}

func TestReadBodyRejectsDeclaredOversizeLength(t *testing.T) {
	resp := &http.Response{
		ContentLength: maxResponseBytes + 1,
		Body:          io.NopCloser(strings.NewReader("short")),
	}
	if _, err := readBody(resp); err == nil {
		t.Fatal("want an error when Content-Length exceeds the limit")
	}
}

func TestReadBodyAllowsNormalPayloads(t *testing.T) {
	resp := &http.Response{ContentLength: -1, Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("readBody: %v", err)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("body = %q", body)
	}

	exact := bytes.Repeat([]byte("a"), maxResponseBytes)
	resp = &http.Response{ContentLength: int64(len(exact)), Body: io.NopCloser(bytes.NewReader(exact))}
	body, err = readBody(resp)
	if err != nil {
		t.Fatalf("readBody at exactly the limit: %v", err)
	}
	if len(body) != maxResponseBytes {
		t.Errorf("body = %d bytes, want the full %d", len(body), maxResponseBytes)
	}
}

type countingStream struct {
	read int
	cap  int
}

func (s *countingStream) Read(p []byte) (int, error) {
	if s.read >= s.cap {
		return 0, errors.New("read past the cap")
	}
	n := len(p)
	if s.read+n > s.cap {
		n = s.cap - s.read
	}
	s.read += n
	return n, nil
}

func TestReadBodyStopsPullingAtTheLimit(t *testing.T) {
	stream := &countingStream{cap: 4 * maxResponseBytes}
	resp := &http.Response{ContentLength: -1, Body: io.NopCloser(stream)}

	if _, err := readBody(resp); err == nil {
		t.Fatal("want an oversize error from an endless body")
	}
	if stream.read > maxResponseBytes+1 {
		t.Errorf("readBody pulled %d bytes from an endless body, want at most %d: the read has to be capped, "+
			"not just measured after the fact", stream.read, maxResponseBytes+1)
	}
}

func TestReadBodyStopsAtLimit(t *testing.T) {
	resp := &http.Response{
		ContentLength: -1,
		Body:          io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("a"), maxResponseBytes+64))),
	}
	if _, err := readBody(resp); err == nil {
		t.Fatal("want an error when an undeclared body exceeds the limit")
	}
}
