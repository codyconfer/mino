package gitlab

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/codyconfer/mino/internal/errs"
)

type recorder struct {
	mu       sync.Mutex
	requests []string
	routes   map[string]string
	status   map[string]int
	headers  map[string]string
}

func newRecorder(routes map[string]string) *recorder {
	return &recorder{routes: routes, status: map[string]int{}}
}

func (r *recorder) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		r.requests = append(r.requests, req.URL.RequestURI())
		r.mu.Unlock()

		for k, v := range r.headers {
			w.Header().Set(k, v)
		}
		path := strings.TrimPrefix(req.URL.EscapedPath(), "/api/v4/")
		if code, ok := r.status[path]; ok {
			w.WriteHeader(code)
			w.Write([]byte(`{"message":"boom"}`))
			return
		}
		body, ok := r.routes[path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"message":"404 Not Found"}`))
			return
		}
		w.Write([]byte(body))
	}
}

func (r *recorder) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.requests...)
}

func (r *recorder) countPath(fragment string) int {
	n := 0
	for _, req := range r.seen() {
		if strings.Contains(req, fragment) {
			n++
		}
	}
	return n
}

func newSignal(t *testing.T, rec *recorder, selectors []string, opts ...Option) *Signal {
	t.Helper()
	b := apiBackend(t, rec.handler())
	s, err := New(selectors, b, 30, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestFetchReturnsOneSectionPerQueryInOrder(t *testing.T) {
	rec := newRecorder(map[string]string{
		"merge_requests": mrFixture,
		"issues":         issueFixture,
	})
	s := newSignal(t, rec, []string{"kind:mr scope:assigned", "kind:issue scope:assigned"})

	secs, err := s.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(secs) != 2 {
		t.Fatalf("got %d sections, want one per query", len(secs))
	}
	if secs[0].Signal != signalName || len(secs[0].Items) != 2 {
		t.Errorf("first section = %+v", secs[0])
	}
	if secs[1].Items[0].Kind != "issue" {
		t.Errorf("second section holds %q, want issues", secs[1].Items[0].Kind)
	}
	if secs[0].Meta["shown"] != "2" {
		t.Errorf("meta = %v, want shown counted", secs[0].Meta)
	}
}

func TestFetchNamesDefaultQueries(t *testing.T) {
	rec := newRecorder(map[string]string{
		"merge_requests": mrFixture,
		"user":           `{"username":"codyconfer"}`,
	})
	s := newSignal(t, rec, nil)

	secs, err := s.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(secs) != 2 {
		t.Fatalf("got %d sections, want the two defaults", len(secs))
	}
	if secs[0].Title != "Assigned Merge Requests" || secs[1].Title != "Review Requests" {
		t.Errorf("titles = %q, %q", secs[0].Title, secs[1].Title)
	}
}

func TestWithTitleNamesASingleQueryOnly(t *testing.T) {
	rec := newRecorder(map[string]string{"merge_requests": mrFixture})

	one := newSignal(t, rec, []string{"kind:mr scope:assigned"}, WithTitle("My work"))
	secs, err := one.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if secs[0].Title != "My work" {
		t.Errorf("title = %q, want the override", secs[0].Title)
	}

	many := newSignal(t, rec, []string{"kind:mr scope:assigned", "kind:mr scope:created"}, WithTitle("My work"))
	secs, err = many.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, sec := range secs {
		if sec.Title == "My work" {
			t.Error("a title override was applied to several sections, so two panels would claim the " +
				"same name for different filters")
		}
	}
}

func TestUntitledSectionFallsBackToTheSelector(t *testing.T) {
	rec := newRecorder(map[string]string{"merge_requests": mrFixture})
	s := newSignal(t, rec, []string{"kind:mr state:merged"})

	secs, err := s.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if secs[0].Title != "kind:mr state:merged" {
		t.Errorf("title = %q, want the raw selector", secs[0].Title)
	}
}

func TestConfiguredViewerSkipsTheUserCall(t *testing.T) {
	rec := newRecorder(map[string]string{"merge_requests": mrFixture})
	s := newSignal(t, rec, []string{"kind:mr reviewer:@me", "kind:mr author:@me"}, WithViewer("acme-bot"))

	if _, err := s.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := rec.countPath("/user"); n != 0 {
		t.Errorf("/user was called %d times with gitlab.viewer set; the configured value works "+
			"offline and for a service token", n)
	}
	for _, req := range rec.seen() {
		if strings.Contains(req, "%40me") || strings.Contains(req, "@me") {
			t.Errorf("request %q still carries @me", req)
		}
		if !strings.Contains(req, "acme-bot") {
			t.Errorf("request %q does not carry the viewer", req)
		}
	}
}

func TestViewerIsResolvedOnceAcrossQueries(t *testing.T) {
	rec := newRecorder(map[string]string{
		"merge_requests": mrFixture,
		"user":           `{"username":"codyconfer"}`,
	})
	s := newSignal(t, rec, []string{"kind:mr reviewer:@me", "kind:mr author:@me", "kind:mr assignee:@me"})

	if _, err := s.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := rec.countPath("/user?"); n > 1 {
		t.Errorf("/user called %d times, want at most once memoized across queries", n)
	}
	if n := rec.countPath("/api/v4/user"); n != 1 {
		t.Errorf("/user called %d times, want exactly 1", n)
	}
}

func TestViewerFailureIsCachedNotRetried(t *testing.T) {
	rec := newRecorder(map[string]string{"merge_requests": mrFixture})
	rec.status["user"] = http.StatusUnauthorized
	s := newSignal(t, rec, []string{"kind:mr reviewer:@me", "kind:mr author:@me"})

	if _, err := s.Fetch(context.Background()); err == nil {
		t.Fatal("an unresolvable viewer produced no error")
	}
	if n := rec.countPath("/api/v4/user"); n != 1 {
		t.Errorf("/user called %d times after failing; a broken credential must not fan out into one "+
			"call per query", n)
	}
}

func TestViewerSubstitutionAlsoRewritesTheTitle(t *testing.T) {
	rec := newRecorder(map[string]string{
		"merge_requests": mrFixture,
		"user":           `{"username":"codyconfer"}`,
	})
	s := newSignal(t, rec, []string{"kind:mr reviewer:@me"})

	secs, err := s.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(secs[0].Title, ViewerAlias) {
		t.Errorf("title = %q still claims @me, which is not what was sent", secs[0].Title)
	}
	if !strings.Contains(secs[0].Title, "codyconfer") {
		t.Errorf("title = %q, want the resolved login", secs[0].Title)
	}
}

func TestFetchErrorNamesTheSelectorAndKeepsTheHint(t *testing.T) {
	rec := newRecorder(map[string]string{})
	rec.status["merge_requests"] = http.StatusUnauthorized
	s := newSignal(t, rec, []string{"kind:mr scope:assigned"})

	_, err := s.Fetch(context.Background())
	if err == nil {
		t.Fatal("a 401 was swallowed")
	}
	if !strings.Contains(err.Error(), "kind:mr scope:assigned") {
		t.Errorf("error = %v, want the selector named so the user knows which panel failed", err)
	}
	if errs.KindOf(err) != errs.KindAuth {
		t.Errorf("kind = %v, want auth preserved through the wrap", errs.KindOf(err))
	}
	if !strings.Contains(errs.Hint(err), "mino login gitlab") {
		t.Errorf("hint = %q, want the inner hint to survive the wrap", errs.Hint(err))
	}
}

func TestNewRejectsABadSelectorAtBuildTime(t *testing.T) {
	_, err := New([]string{"kind:mr nonesuch:x"}, nil, 30)
	if err == nil {
		t.Fatal("a bad selector built a signal; it would render as an empty section at fetch time " +
			"instead of a config error at build time")
	}
	if errs.KindOf(err) != errs.KindConfig {
		t.Errorf("kind = %v, want config", errs.KindOf(err))
	}
}

func TestSinceIsAppliedToEveryQuery(t *testing.T) {
	rec := newRecorder(map[string]string{"merge_requests": mrFixture})
	s := newSignal(t, rec, []string{"kind:mr scope:assigned", "kind:mr scope:created"})
	s.setSince("2026-08-01T00:00:00Z")

	if _, err := s.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, req := range rec.seen() {
		if !strings.Contains(req, "updated_after=2026-08-01") {
			t.Errorf("request %q carries no cursor; the stream would refetch everything each tick", req)
		}
	}
}

func TestFetchDoesNotMutateTheStoredSelector(t *testing.T) {
	rec := newRecorder(map[string]string{
		"merge_requests": mrFixture,
		"user":           `{"username":"codyconfer"}`,
	})
	s := newSignal(t, rec, []string{"kind:mr reviewer:@me"})

	if _, err := s.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := s.queries[0].sel.Params.Get("reviewer_username"); got != ViewerAlias {
		t.Errorf("stored selector param = %q, want the unresolved %s; resolving in place would bake "+
			"one run's login into every later fetch", got, ViewerAlias)
	}
	if !s.queries[0].sel.NeedsViewer() {
		t.Error("the stored selector lost its viewer marker")
	}
}

func TestDefaultQueriesAreCopied(t *testing.T) {
	a := DefaultQueries()
	a[0] = "mutated"
	if DefaultQueries()[0] == "mutated" {
		t.Error("DefaultQueries hands out the package slice")
	}
}
