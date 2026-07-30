package github

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/codyconfer/munin/internal/signals"
)

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
