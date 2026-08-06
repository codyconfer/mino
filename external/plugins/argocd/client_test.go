package argocd

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestListSendsBearerAndFilters(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		serveJSON(w, http.StatusOK, []byte(`{"items":[]}`))
	})
	cfg := fs.config()
	cfg.App = "payments-api"
	cfg.Projects = []string{"platform", "storefront"}
	cfg.Selector = "env=prod"
	cfg.AppNamespace = "team-search"

	if _, _, err := fs.client(cfg).Applications(context.Background()); err != nil {
		t.Fatalf("Applications: %v", err)
	}

	req := fs.last()
	if req.auth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want a bearer token", req.auth)
	}
	if req.path != "/api/v1/applications" {
		t.Errorf("path = %q, want /api/v1/applications", req.path)
	}
	if got := req.query["project"]; len(got) != 2 || got[0] != "platform" || got[1] != "storefront" {
		t.Errorf("project = %v, want both projects as repeated params; ArgoCD reads project, not projects", got)
	}
	if got := req.query.Get("selector"); got != "env=prod" {
		t.Errorf("selector = %q, want env=prod", got)
	}
	if got := req.query.Get("appNamespace"); got != "team-search" {
		t.Errorf("appNamespace = %q, want team-search", got)
	}
	if got := req.query.Get("name"); got != "payments-api" {
		t.Errorf("name = %q, want payments-api", got)
	}
}

func TestRefreshIsNeverSent(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		serveJSON(w, http.StatusOK, fixture(t, "applications.json"))
	})
	cfg := fs.config()

	if _, _, err := fs.client(cfg).Applications(context.Background()); err != nil {
		t.Fatalf("Applications: %v", err)
	}
	if _, err := fs.client(cfg).Application(context.Background(), "payments-api", ""); err != nil {
		t.Fatalf("Application: %v", err)
	}

	fs.mu.Lock()
	defer fs.mu.Unlock()
	for _, req := range fs.requests {
		if _, present := req.query["refresh"]; present {
			t.Fatalf("%s sent refresh=%v; that forces a reconcile against the user's cluster, which is a "+
				"write-ish side effect this plugin promises never to cause", req.path, req.query["refresh"])
		}
	}
}

func TestAppNamespaceThreadedOntoPerAppCalls(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		serveJSON(w, http.StatusOK, []byte(`{}`))
	})
	c := fs.client(fs.config())

	if _, err := c.Application(context.Background(), "search-indexer", "team-search"); err != nil {
		t.Fatalf("Application: %v", err)
	}
	if got := fs.last().query.Get("appNamespace"); got != "team-search" {
		t.Errorf("appNamespace = %q; without it an app outside the argocd namespace 404s", got)
	}

	if _, err := c.ResourceTree(context.Background(), "search-indexer", "team-search"); err != nil {
		t.Fatalf("ResourceTree: %v", err)
	}
	if got := fs.last().path; got != "/api/v1/applications/search-indexer/resource-tree" {
		t.Errorf("resource tree path = %q", got)
	}

	if _, err := c.RevisionMetadata(context.Background(), "search-indexer", "team-search", "abc123"); err != nil {
		t.Fatalf("RevisionMetadata: %v", err)
	}
	if got := fs.last().path; got != "/api/v1/applications/search-indexer/revisions/abc123/metadata" {
		t.Errorf("revision metadata path = %q", got)
	}
}

func TestStatusClassification(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		wantText string
		wantHint string
	}{
		{"unauthorized", http.StatusUnauthorized, `{"error":"token signature is invalid"}`,
			"unauthorized", "generate-token"},
		{"forbidden", http.StatusForbidden, "",
			"forbidden", "applications, get"},
		{"not found", http.StatusNotFound, `{"error":"applications.argoproj.io \"nope\" not found"}`,
			"not found", "app_namespace"},
		{"rate limited", http.StatusTooManyRequests, `{"error":"slow down"}`,
			"rate limited", "Retry-After"},
		{"server error", http.StatusInternalServerError, `{"error":"boom"}`,
			"server error", "unhealthy"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := c.body
			if body == "" {
				body = string(fixture(t, "error_403.json"))
			}
			fs := newFakeServer(t, func(w http.ResponseWriter, _ *http.Request) {
				serveJSON(w, c.status, []byte(body))
			})
			_, _, err := fs.client(fs.config()).Applications(context.Background())
			if err == nil {
				t.Fatalf("status %d produced no error", c.status)
			}
			if !strings.Contains(err.Error(), c.wantText) {
				t.Errorf("error = %q, want it to say %q", err, c.wantText)
			}
			if !strings.Contains(err.Error(), c.wantHint) {
				t.Errorf("error = %q, want the hint to mention %q", err, c.wantHint)
			}
		})
	}
}

func TestForbiddenPreservesTheRBACTriple(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		serveJSON(w, http.StatusForbidden, fixture(t, "error_403.json"))
	})
	_, _, err := fs.client(fs.config()).Applications(context.Background())
	if err == nil {
		t.Fatal("403 produced no error")
	}
	if !strings.Contains(err.Error(), "permission denied: applications, get, platform/payments-api") {
		t.Errorf("error = %q, want the server message verbatim; it names the exact resource/action/object "+
			"the RBAC policy has to allow, which is the whole fix", err)
	}
}

func TestMalformedJSONIsReportedWithAnExcerpt(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		serveJSON(w, http.StatusOK, []byte(`<html>login page</html>`))
	})
	_, _, err := fs.client(fs.config()).Applications(context.Background())
	if err == nil {
		t.Fatal("an HTML body decoded as JSON without error")
	}
	if !strings.Contains(err.Error(), "login page") {
		t.Errorf("error = %q, want an excerpt of what the server actually returned", err)
	}
}

func TestOversizeBodyIsRejected(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", "999999999")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"items":[]}`))
	})
	_, _, err := fs.client(fs.config()).Applications(context.Background())
	if err == nil {
		t.Fatal("an oversize response was accepted; a hostile or broken server could exhaust memory")
	}
	if !strings.Contains(err.Error(), "limit") {
		t.Errorf("error = %q, want it to name the size limit", err)
	}
}

func TestMissingTokenFailsBeforeAnyRequest(t *testing.T) {
	t.Setenv(DefaultTokenEnv, "")
	fs := newFakeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		serveJSON(w, http.StatusOK, []byte(`{"items":[]}`))
	})
	c := NewClient(fs.config(), staticTokens{})
	c.HTTP = fs.srv.Client()

	if _, _, err := c.Applications(context.Background()); err == nil {
		t.Fatal("a tokenless client issued a request")
	}
	if fs.count() != 0 {
		t.Errorf("%d requests reached the server; an unauthenticated call is pointless traffic", fs.count())
	}
}
