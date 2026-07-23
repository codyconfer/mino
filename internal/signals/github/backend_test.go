package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
