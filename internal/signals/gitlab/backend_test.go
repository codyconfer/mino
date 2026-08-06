package gitlab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/errs"
)

type staticToken string

func (s staticToken) Token(context.Context) (string, error) { return string(s), nil }

func apiBackend(t *testing.T, h http.HandlerFunc) APIBackend {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return APIBackend{Auth: staticToken("glpat-secret"), BaseURL: srv.URL + "/api/v4"}
}

func TestNormalizeAPIURL(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "", want: ""},
		{in: "https://gitlab.com", want: "https://gitlab.com/api/v4"},
		{in: "https://gitlab.example.com/", want: "https://gitlab.example.com/api/v4"},
		{in: "https://gitlab.example.com/api/v4", want: "https://gitlab.example.com/api/v4"},
		{in: "https://gitlab.example.com/api/v4/", want: "https://gitlab.example.com/api/v4"},
		{in: "http://gitlab.example.com", wantErr: true},
		{in: "gitlab.example.com", wantErr: true},
		{in: "https://", wantErr: true},
	}
	for _, c := range cases {
		got, err := NormalizeAPIURL(c.in)
		if (err != nil) != c.wantErr {
			t.Fatalf("NormalizeAPIURL(%q) err = %v, wantErr %v", c.in, err, c.wantErr)
		}
		if err == nil && got != c.want {
			t.Errorf("NormalizeAPIURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestAPIBackendEscapesNestedProjectPaths(t *testing.T) {
	var gotURI, gotAuth string
	b := apiBackend(t, func(w http.ResponseWriter, r *http.Request) {
		gotURI, gotAuth = r.RequestURI, r.Header.Get("Authorization")
		w.Write([]byte(`[]`))
	})

	sel, err := ParseSelector("kind:mr project:group/sub/proj state:opened")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Get(context.Background(), sel.Path(), sel.Query()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotURI, "group%2Fsub%2Fproj") {
		t.Errorf("request URI = %q, want the project path percent-encoded; net/url decodes Path and "+
			"keeps the escape only in RawPath, so any reassembly collapses it and hits the wrong "+
			"endpoint", gotURI)
	}
	if gotAuth != "Bearer glpat-secret" {
		t.Errorf("Authorization = %q, want bearer", gotAuth)
	}
}

func TestAPIBackendLeavesNumericProjectIDsAlone(t *testing.T) {
	var gotURI string
	b := apiBackend(t, func(w http.ResponseWriter, r *http.Request) {
		gotURI = r.RequestURI
		w.Write([]byte(`[]`))
	})

	sel, _ := ParseSelector("kind:pipeline project:12345")
	if _, err := b.Get(context.Background(), sel.Path(), sel.Query()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotURI, "/projects/12345/pipelines") {
		t.Errorf("request URI = %q, want the numeric id verbatim", gotURI)
	}
}

func TestAPIBackendReadsPagingHeaders(t *testing.T) {
	b := apiBackend(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Next-Page", "2")
		w.Header().Set("X-Total", "57")
		w.Header().Set("X-Total-Pages", "3")
		w.Write([]byte(`[{},{}]`))
	})

	p, err := b.Get(context.Background(), "merge_requests", url.Values{"per_page": {"2"}})
	if err != nil {
		t.Fatal(err)
	}
	if p.NextPage != 2 || p.Total != 57 || !p.HasTotal || p.TotalPages != 3 {
		t.Errorf("page = %+v, want the X- headers parsed", p)
	}
	if p.Short {
		t.Error("a full page was reported short")
	}
}

func TestAPIBackendClassifiesAScopedNotFound(t *testing.T) {
	b := apiBackend(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"404 Project Not Found"}`))
	})

	_, err := b.Get(context.Background(), "projects/acme%2Fapi/merge_requests", nil)
	if err == nil {
		t.Fatal("a 404 was accepted")
	}
	if errs.KindOf(err) != errs.KindUsage {
		t.Errorf("kind = %v, want usage", errs.KindOf(err))
	}
	if !strings.Contains(errs.Hint(err), "cannot see") {
		t.Errorf("hint = %q; GitLab returns 404 rather than 403 for projects a token cannot see, so "+
			"the naive \"you typed it wrong\" reading is right only half the time", errs.Hint(err))
	}
}

func TestAPIBackendLeavesAnUnscopedNotFoundAlone(t *testing.T) {
	b := apiBackend(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"404 Not Found"}`))
	})

	_, err := b.Get(context.Background(), "merge_requests", nil)
	if errs.KindOf(err) != errs.KindSignal {
		t.Errorf("kind = %v, want signal; an unscoped 404 is a routing bug, not a permission "+
			"problem the user can fix", errs.KindOf(err))
	}
}

func TestAPIBackendRecordsARateHint(t *testing.T) {
	freezeClock(t, "2026-08-05T12:00:00Z")
	rate := &RateHint{}
	b := apiBackend(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`Retry later`))
	})
	b.Rate = rate

	if _, err := b.Get(context.Background(), "merge_requests", nil); err == nil {
		t.Fatal("a 429 was accepted")
	}
	if got := rate.delay(timeNow()); got.Seconds() != 120 {
		t.Errorf("rate hint = %v, want 120s so the poller can push its next tick out", got)
	}
}

func TestAPIBackendNeedsAuth(t *testing.T) {
	b := APIBackend{BaseURL: "https://gitlab.example.com/api/v4"}
	if _, err := b.Get(context.Background(), "merge_requests", nil); err == nil {
		t.Fatal("a backend with no auth made a request")
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
}

func TestCLIBackendBuildsTheExactArgv(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	fakeGLab(t, "#!/bin/sh\necho \"$@\" > \""+argsFile+"\"\necho '[]'\n")

	b := CLIBackend{Hostname: "gitlab.example.com"}
	sel, _ := ParseSelector("kind:mr project:group/sub/proj state:opened")
	if _, err := b.Get(context.Background(), sel.Path(), sel.Query()); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(string(raw))
	want := "api --hostname gitlab.example.com -X GET projects/group%2Fsub%2Fproj/merge_requests?state=opened"
	if got != want {
		t.Errorf("glab argv = %q, want %q", got, want)
	}
}

func TestCLIBackendOmitsTheHostnameWhenUnset(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	fakeGLab(t, "#!/bin/sh\necho \"$@\" > \""+argsFile+"\"\necho '[]'\n")

	if _, err := (CLIBackend{}).Get(context.Background(), "merge_requests", nil); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(argsFile)
	if got := strings.TrimSpace(string(raw)); got != "api -X GET merge_requests" {
		t.Errorf("glab argv = %q", got)
	}
}

func TestCLIBackendTranslatesGLabFailures(t *testing.T) {
	cases := []struct {
		name     string
		stderr   string
		wantKind errs.Kind
		wantHint string
	}{
		{"scope", "insufficient_scope", errs.KindAuth, "read_api"},
		{"unauthorized", "401 Unauthorized", errs.KindAuth, "mino login gitlab"},
		{"not found", "404 Not Found", errs.KindUsage, "cannot see"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeGLab(t, "#!/bin/sh\necho '"+c.stderr+"' >&2\nexit 1\n")
			_, err := (CLIBackend{}).Get(context.Background(), "merge_requests", nil)
			if err == nil {
				t.Fatal("a glab failure was accepted")
			}
			if errs.KindOf(err) != c.wantKind {
				t.Errorf("kind = %v, want %v (%v)", errs.KindOf(err), c.wantKind, err)
			}
			if !strings.Contains(errs.Hint(err), c.wantHint) {
				t.Errorf("hint = %q, want it to mention %q", errs.Hint(err), c.wantHint)
			}
		})
	}
}

func TestProjectPathFromWebURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://gitlab.com/acme/api/-/merge_requests/42", "acme/api"},
		{"https://gitlab.com/acme/platform/sub/api/-/issues/7", "acme/platform/sub/api"},
		{"https://gitlab.example.com/acme/api", "acme/api"},
		{"", ""},
	}
	for _, c := range cases {
		if got := projectPathFromWebURL(c.in); got != c.want {
			t.Errorf("projectPathFromWebURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
