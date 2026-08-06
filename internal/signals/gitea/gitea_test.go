package gitea

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/signals"
)

const issuesFixture = `[
  {"number":12,"title":"fix the flaky test","body":"it flakes","html_url":"https://git.example.com/acme/tools/pulls/12",
   "state":"open","comments":3,"updated_at":"2026-07-20T15:04:05Z","user":{"login":"alice"},
   "labels":[{"name":"bug"},{"name":"ci"}],"assignees":[{"login":"bob"}],
   "repository":{"full_name":"acme/tools","owner":"acme","name":"tools"},
   "pull_request":{"merged":false,"draft":true}},
  {"number":9,"title":"cannot log in","body":"500 on submit","html_url":"https://git.example.com/acme/tools/issues/9",
   "state":"open","updated_at":"2026-07-19T09:00:00Z","user":{"login":"carol"},
   "repository":{"owner":"acme","name":"tools"}}
]`

type fakeBackend struct {
	search    func(q Query, limit int) (Result, error)
	whoami    func() ([]byte, error)
	whoamiHit int

	mu sync.Mutex

	issue         []byte
	comments      []byte
	pull          []byte
	reviews       []byte
	commentsPage  int
	commentsLimit int
	calls         []string
}

func (f *fakeBackend) record(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, name)
}

func (f *fakeBackend) SearchIssues(_ context.Context, q Query, limit int) (Result, error) {
	f.record("search")
	if f.search == nil {
		return Result{Body: []byte(issuesFixture)}, nil
	}
	return f.search(q, limit)
}

func (f *fakeBackend) Issue(_ context.Context, _ Ref) ([]byte, error) {
	f.record("issue")
	return f.issue, nil
}

func (f *fakeBackend) IssueComments(_ context.Context, _ Ref, page, limit int) ([]byte, error) {
	f.record("comments")
	f.mu.Lock()
	f.commentsPage, f.commentsLimit = page, limit
	f.mu.Unlock()
	return f.comments, nil
}

func (f *fakeBackend) PullRequest(_ context.Context, _ Ref) ([]byte, error) {
	f.record("pull")
	return f.pull, nil
}

func (f *fakeBackend) PullReviews(_ context.Context, _ Ref, _ int) ([]byte, error) {
	f.record("reviews")
	return f.reviews, nil
}

func (f *fakeBackend) Whoami(context.Context) ([]byte, error) {
	f.record("whoami")
	f.whoamiHit++
	if f.whoami == nil {
		return []byte(`{"login":"alice"}`), nil
	}
	return f.whoami()
}

func (f *fakeBackend) count(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == name {
			n++
		}
	}
	return n
}

func newSignal(t *testing.T, queries []string, b Backend, opts ...Option) signals.Signal {
	t.Helper()
	s, err := New(queries, b, 30, opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestMapIssuesDerivesKindFromPullRequest(t *testing.T) {
	sec, err := mapIssues([]byte(issuesFixture), "review requests", 0, 30)
	if err != nil {
		t.Fatal(err)
	}
	if sec.Signal != "gitea" || sec.Title != "review requests" {
		t.Errorf("section = %+v, want the gitea signal and the given title", sec)
	}
	if len(sec.Items) != 2 {
		t.Fatalf("got %d items, want 2", len(sec.Items))
	}
	pr, issue := sec.Items[0], sec.Items[1]
	if pr.Kind != "pr" || issue.Kind != "issue" {
		t.Errorf("kinds = %q/%q, want pr/issue from the pull_request field", pr.Kind, issue.Kind)
	}
	if pr.Subtitle != "acme/tools" || issue.Subtitle != "acme/tools" {
		t.Errorf("subtitles = %q/%q, want the repo slug from either full_name or owner+name", pr.Subtitle, issue.Subtitle)
	}
	if pr.URL != "https://git.example.com/acme/tools/pulls/12" {
		t.Errorf("URL = %q, want the browsable html_url", pr.URL)
	}
	if pr.Timestamp.IsZero() {
		t.Error("timestamp was not parsed from updated_at")
	}
	for key, want := range map[string]string{
		"number": "12", "state": "open", "author": "alice",
		"repo": "acme/tools", "comments": "3", "labels": "bug,ci", "assignees": "bob", "draft": "true",
	} {
		if got := pr.Meta[key]; got != want {
			t.Errorf("meta[%q] = %q, want %q", key, got, want)
		}
	}
}

func TestIssuesMetaReportsWhatTheHeaderAllows(t *testing.T) {
	cases := []struct {
		name          string
		shown, total  int
		limit         int
		wantMore      string
		wantTruncated bool
	}{
		{name: "total beyond the page", shown: 2, total: 437, limit: 30, wantMore: "435"},
		{name: "total equals the page", shown: 2, total: 2, limit: 30},
		{name: "no total on a full page", shown: 30, limit: 30, wantTruncated: true},
		{name: "no total on a partial page", shown: 2, limit: 30},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			meta := issuesMeta(c.shown, c.total, c.limit, "q")
			if meta[signals.MetaMore] != c.wantMore {
				t.Errorf("more = %q, want %q", meta[signals.MetaMore], c.wantMore)
			}
			truncated := meta[signals.MetaTruncated] == "true"
			if truncated != c.wantTruncated {
				t.Errorf("truncated = %v, want %v; a full page with no X-Total-Count cannot be claimed as the whole answer", truncated, c.wantTruncated)
			}
			if truncated && meta["truncated_reason"] == "" {
				t.Error("truncated with no reason")
			}
		})
	}
}

func TestMapIssuesAcceptsAnEmptyArray(t *testing.T) {
	sec, err := mapIssues([]byte(`[]`), "none", 0, 30)
	if err != nil {
		t.Fatalf("an empty result is not an error: %v", err)
	}
	if len(sec.Items) != 0 || sec.Meta["shown"] != "0" {
		t.Errorf("section = %+v, want an empty section", sec)
	}
}

func TestNewRejectsABadQueryAtConstruction(t *testing.T) {
	_, err := New([]string{"stat:open"}, &fakeBackend{}, 30)
	if err == nil {
		t.Fatal("a bad query expression was accepted; it must fail when the signal is built, not as an empty section later")
	}
	if errs.KindOf(err) != errs.KindConfig {
		t.Errorf("kind = %v, want %v", errs.KindOf(err), errs.KindConfig)
	}
}

func TestFetchUsesTheDefaultQueriesWhenNoneAreConfigured(t *testing.T) {
	b := &fakeBackend{}
	secs, err := newSignal(t, nil, b).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(secs) != 2 {
		t.Fatalf("got %d sections, want the two default queries", len(secs))
	}
	if secs[0].Title != "Open Pull Requests" || secs[1].Title != "Review Requests" {
		t.Errorf("titles = %q/%q, want the default names", secs[0].Title, secs[1].Title)
	}
}

func TestWithTitleNamesASingleQueryOnly(t *testing.T) {
	b := &fakeBackend{}
	secs, err := newSignal(t, []string{"type:pulls"}, b, WithTitle("mine")).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if secs[0].Title != "mine" {
		t.Errorf("title = %q, want the configured title", secs[0].Title)
	}

	secs, err = newSignal(t, []string{"type:pulls", "type:issues"}, b, WithTitle("mine")).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, sec := range secs {
		if sec.Title == "mine" {
			t.Error("one title was applied to several queries; each section must say which query produced it")
		}
	}
}

func TestFetchNamesTheQueryThatFailed(t *testing.T) {
	b := &fakeBackend{search: func(Query, int) (Result, error) {
		return Result{}, errs.New(errs.KindAuth, "gitea api 401 Unauthorized").WithHint("check the token")
	}}
	_, err := newSignal(t, []string{"type:pulls state:open"}, b).Fetch(context.Background())
	if err == nil {
		t.Fatal("Fetch swallowed a backend failure")
	}
	if !strings.Contains(err.Error(), `query "type:pulls state:open"`) {
		t.Errorf("error = %q, want it to name the query", err)
	}
	if errs.Hint(err) != "check the token" {
		t.Errorf("hint = %q, want the backend hint carried through", errs.Hint(err))
	}
	if errs.KindOf(err) != errs.KindAuth {
		t.Errorf("kind = %v, want the backend kind preserved", errs.KindOf(err))
	}
}

func TestFetchResolvesTheViewerOnlyWhereGiteaNeedsALogin(t *testing.T) {
	b := &fakeBackend{}
	if _, err := newSignal(t, []string{"type:pulls review_requested:@me", "type:pulls created:@me"}, b).Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if b.whoamiHit != 0 {
		t.Errorf("resolved @me %d time(s) for cross-repo actor filters; gitea already scopes those to the token's user", b.whoamiHit)
	}

	b = &fakeBackend{}
	if _, err := newSignal(t, []string{"owner:@me", "repo:acme/tools created:@me"}, b).Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if b.whoamiHit != 1 {
		t.Errorf("resolved @me %d time(s), want exactly one lookup shared by both queries", b.whoamiHit)
	}
}

func TestFetchPrefersTheConfiguredViewer(t *testing.T) {
	b := &fakeBackend{search: func(q Query, _ int) (Result, error) {
		if got := q.Values(30, 1).Get("owner"); got != "acme-bot" {
			t.Errorf("owner = %q, want the configured viewer", got)
		}
		return Result{Body: []byte(`[]`)}, nil
	}}
	if _, err := newSignal(t, []string{"owner:@me"}, b, WithViewer("acme-bot")).Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if b.whoamiHit != 0 {
		t.Error("asked the instance who @me is even though gitea.viewer is set")
	}
}

func TestFetchExplainsAnUnresolvableViewer(t *testing.T) {
	b := &fakeBackend{whoami: func() ([]byte, error) { return []byte(`{}`), nil }}
	_, err := newSignal(t, []string{"owner:@me"}, b).Fetch(context.Background())
	if err == nil {
		t.Fatal("an unresolvable @me was accepted")
	}
	if !strings.Contains(errs.Hint(err), "gitea.viewer") {
		t.Errorf("hint = %q, want it to name gitea.viewer", errs.Hint(err))
	}
}
