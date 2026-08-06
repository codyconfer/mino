package gitea

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/auth"
	"github.com/codyconfer/mino/internal/errs"
)

type recordedRequest struct {
	path   string
	query  url.Values
	auth   string
	accept string
}

func newBackendServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) (APIBackend, *[]recordedRequest) {
	t.Helper()
	var seen []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, recordedRequest{
			path:   r.URL.Path,
			query:  r.URL.Query(),
			auth:   r.Header.Get("Authorization"),
			accept: r.Header.Get("Accept"),
		})
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return APIBackend{Auth: auth.StaticGiteaToken("tok"), HTTP: srv.Client(), BaseURL: srv.URL + "/api/v1"}, &seen
}

func TestSearchIssuesSendsTheTokenSchemeAndTypedParams(t *testing.T) {
	b, seen := newBackendServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Total-Count", "437")
		_, _ = w.Write([]byte(issuesFixture))
	})

	res, err := b.SearchIssues(context.Background(), mustParse(t, "type:pulls review_requested:@me"), 30)
	if err != nil {
		t.Fatalf("SearchIssues: %v", err)
	}
	if res.Total != 437 {
		t.Errorf("Total = %d, want 437 from X-Total-Count; the response body is a bare array with no count in it", res.Total)
	}
	req := (*seen)[0]
	if req.path != "/api/v1/repos/issues/search" {
		t.Errorf("path = %q, want the cross-repo search endpoint", req.path)
	}
	if req.auth != "token tok" {
		t.Errorf("Authorization = %q, want %q: Bearer only works on recent Gitea versions", req.auth, "token tok")
	}
	if req.accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", req.accept)
	}
	for key, want := range map[string]string{"type": "pulls", "review_requested": "true", "state": "open", "limit": "30", "page": "1"} {
		if got := req.query.Get(key); got != want {
			t.Errorf("query[%q] = %q, want %q", key, got, want)
		}
	}
}

func TestTotalCountIsOptional(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   int
	}{
		{"absent", "", 0},
		{"garbage", "many", 0},
		{"negative", "-4", 0},
		{"present", "12", 12},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hdr := http.Header{}
			if c.header != "" {
				hdr.Set("X-Total-Count", c.header)
			}
			if got := totalCount(hdr); got != c.want {
				t.Errorf("totalCount(%q) = %d, want %d", c.header, got, c.want)
			}
		})
	}
}

func TestBackendPathsAndEscaping(t *testing.T) {
	b, seen := newBackendServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	ctx := context.Background()
	ref := Ref{Owner: "acme corp", Repo: "tools", Number: 12}

	if _, err := b.Issue(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if _, err := b.IssueComments(ctx, ref, 3, 20); err != nil {
		t.Fatal(err)
	}
	if _, err := b.PullRequest(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if _, err := b.PullReviews(ctx, ref, 20); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Whoami(ctx); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"/api/v1/repos/acme corp/tools/issues/12",
		"/api/v1/repos/acme corp/tools/issues/12/comments",
		"/api/v1/repos/acme corp/tools/pulls/12",
		"/api/v1/repos/acme corp/tools/pulls/12/reviews",
		"/api/v1/user",
	}
	for i, path := range want {
		if (*seen)[i].path != path {
			t.Errorf("request %d path = %q, want %q", i, (*seen)[i].path, path)
		}
	}
	comments := (*seen)[1].query
	if comments.Get("page") != "3" || comments.Get("limit") != "20" {
		t.Errorf("comments query = %v, want page 3 limit 20", comments)
	}
}

func TestBackendStatusMapping(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		header   http.Header
		wantKind errs.Kind
		wantHint string
	}{
		{name: "unauthorized", status: 401, body: `{"message":"unauthorized"}`, wantKind: errs.KindAuth, wantHint: "Settings -> Applications"},
		{name: "forbidden", status: 403, body: `{"message":"forbidden"}`, wantKind: errs.KindAuth, wantHint: "credential may lack"},
		{name: "not found", status: 404, body: `{"message":"Not Found"}`, wantKind: errs.KindSignal, wantHint: "does not expose that endpoint"},
		{name: "web ui", status: 404, body: `<!DOCTYPE html><html><body>404</body></html>`, wantKind: errs.KindConfig, wantHint: "mino appends /api/v1"},
		{name: "rate limited", status: 429, body: `too many requests`, header: http.Header{"Retry-After": {"60"}}, wantKind: errs.KindSignal, wantHint: "retry after 1m0s"},
		{name: "server error", status: 500, body: `boom`, wantKind: errs.KindSignal},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, _ := newBackendServer(t, func(w http.ResponseWriter, _ *http.Request) {
				for k, vals := range c.header {
					w.Header().Set(k, vals[0])
				}
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(c.body))
			})
			_, err := b.SearchIssues(context.Background(), mustParse(t, ""), 30)
			if err == nil {
				t.Fatalf("status %d was treated as success", c.status)
			}
			if errs.KindOf(err) != c.wantKind {
				t.Errorf("kind = %v, want %v", errs.KindOf(err), c.wantKind)
			}
			if c.wantHint != "" && !strings.Contains(errs.Hint(err), c.wantHint) {
				t.Errorf("hint = %q, want it to contain %q", errs.Hint(err), c.wantHint)
			}
		})
	}
}

func TestBackendRefusesToRunWithoutACredential(t *testing.T) {
	b := APIBackend{BaseURL: "https://git.example.com/api/v1"}
	if _, err := b.SearchIssues(context.Background(), mustParse(t, ""), 30); errs.KindOf(err) != errs.KindAuth {
		t.Errorf("err = %v, want an auth error when no token source is wired", err)
	}
}

func TestNormalizeAPIURL(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"empty", "", "", false},
		{"root gets the api path", "https://git.example.com", "https://git.example.com/api/v1", false},
		{"trailing slash", "https://git.example.com/", "https://git.example.com/api/v1", false},
		{"already the api base", "https://git.example.com/api/v1", "https://git.example.com/api/v1", false},
		{"api base with slash", "https://git.example.com/api/v1/", "https://git.example.com/api/v1", false},
		{"subpath install", "https://example.com/gitea", "https://example.com/gitea/api/v1", false},
		{"localhost http", "http://localhost:3000", "http://localhost:3000/api/v1", false},
		{"loopback http", "http://127.0.0.1:3000/api/v1", "http://127.0.0.1:3000/api/v1", false},
		{"remote http rejected", "http://git.example.com", "", true},
		{"scheme-less rejected", "git.example.com", "", true},
		{"no host rejected", "https://", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := NormalizeAPIURL(c.in)
			if (err != nil) != c.wantErr {
				t.Fatalf("NormalizeAPIURL(%q) err = %v, wantErr %v", c.in, err, c.wantErr)
			}
			if !c.wantErr && got != c.want {
				t.Errorf("NormalizeAPIURL(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
