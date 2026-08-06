package gitlab

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/errs"
)

func freezeClock(t *testing.T, iso string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		t.Fatal(err)
	}
	prev := timeNow
	timeNow = func() time.Time { return ts }
	t.Cleanup(func() { timeNow = prev })
	return ts
}

func TestParseSelectorBuildsPathsAndParams(t *testing.T) {
	freezeClock(t, "2026-08-05T12:00:00Z")

	cases := []struct {
		name      string
		raw       string
		wantPath  string
		wantQuery url.Values
	}{
		{
			name: "defaults to merge requests", raw: "",
			wantPath: "merge_requests", wantQuery: url.Values{},
		},
		{
			name: "assigned open mrs", raw: "kind:mr scope:assigned state:opened",
			wantPath:  "merge_requests",
			wantQuery: url.Values{"scope": {"assigned_to_me"}, "state": {"opened"}},
		},
		{
			name: "scope alias mine", raw: "scope:mine",
			wantPath: "merge_requests", wantQuery: url.Values{"scope": {"assigned_to_me"}},
		},
		{
			name: "scope literal passes through", raw: "scope:created_by_me",
			wantPath: "merge_requests", wantQuery: url.Values{"scope": {"created_by_me"}},
		},
		{
			name: "group target", raw: "group:acme/platform state:opened",
			wantPath:  "groups/acme%2Fplatform/merge_requests",
			wantQuery: url.Values{"state": {"opened"}},
		},
		{
			name: "nested project target", raw: "kind:issue project:acme/platform/api",
			wantPath: "projects/acme%2Fplatform%2Fapi/issues", wantQuery: url.Values{},
		},
		{
			name: "numeric project id is not escaped", raw: "kind:issue project:12345",
			wantPath: "projects/12345/issues", wantQuery: url.Values{},
		},
		{
			name: "pipelines", raw: "kind:pipeline project:acme/api status:failed ref:main",
			wantPath:  "projects/acme%2Fapi/pipelines",
			wantQuery: url.Values{"status": {"failed"}, "ref": {"main"}},
		},
		{
			name: "repeated labels join", raw: "label:backend label:urgent",
			wantPath: "merge_requests", wantQuery: url.Values{"labels": {"backend,urgent"}},
		},
		{
			name: "quoted value keeps spaces", raw: `milestone:"24.9 hardening"`,
			wantPath: "merge_requests", wantQuery: url.Values{"milestone": {"24.9 hardening"}},
		},
		{
			name: "draft maps to wip", raw: "draft:false",
			wantPath: "merge_requests", wantQuery: url.Values{"wip": {"no"}},
		},
		{
			name: "sort expands to two params", raw: "sort:created",
			wantPath:  "merge_requests",
			wantQuery: url.Values{"order_by": {"created_at"}, "sort": {"desc"}},
		},
		{
			name: "since duration resolves against the clock", raw: "since:24h",
			wantPath:  "merge_requests",
			wantQuery: url.Values{"updated_after": {"2026-08-04T12:00:00Z"}},
		},
		{
			name: "since date", raw: "since:2026-01-02",
			wantPath:  "merge_requests",
			wantQuery: url.Values{"updated_after": {"2026-01-02T00:00:00Z"}},
		},
		{
			name: "search is the escape hatch", raw: "search:flaky",
			wantPath: "merge_requests", wantQuery: url.Values{"search": {"flaky"}},
		},
		{
			name: "reviewer on mrs", raw: "reviewer:codyconfer",
			wantPath: "merge_requests", wantQuery: url.Values{"reviewer_username": {"codyconfer"}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, err := ParseSelector(c.raw)
			if err != nil {
				t.Fatalf("ParseSelector(%q): %v", c.raw, err)
			}
			if got := s.Path(); got != c.wantPath {
				t.Errorf("path = %q, want %q", got, c.wantPath)
			}
			if got := s.Query().Encode(); got != c.wantQuery.Encode() {
				t.Errorf("query = %q, want %q", got, c.wantQuery.Encode())
			}
		})
	}
}

func TestParseSelectorRejectsBadInput(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		wantSaid string
	}{
		{"unknown key", "kind:mr nonesuch:x", "nonesuch"},
		{"bare term", "kind:mr open", "open"},
		{"unknown kind", "kind:snippet", "snippet"},
		{"reviewer on issues", "kind:issue reviewer:@me", "kind:mr"},
		{"merged state on issues", "kind:issue state:merged", "kind:issue"},
		{"pipeline without project", "kind:pipeline status:failed", "project:"},
		{"pipeline in a group", "kind:pipeline group:acme", "group"},
		{"status on mrs", "kind:mr status:failed", "kind:pipeline"},
		{"two targets", "project:a/b group:c", "one target"},
		{"empty value", "kind:mr label:", "needs a value"},
		{"bad since", "since:soon", "duration"},
		{"unbalanced quote", `milestone:"24.9`, "unbalanced"},
		{"unknown scope value", "kind:mr scope:everything", "assigned_to_me"},
		{"unknown draft value", "draft:maybe", "true, false"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseSelector(c.raw)
			if err == nil {
				t.Fatalf("ParseSelector(%q) was accepted; an unsupported term must be a config error, "+
					"not a silently-ignored text term", c.raw)
			}
			if errs.KindOf(err) != errs.KindConfig {
				t.Errorf("kind = %v, want config", errs.KindOf(err))
			}
			if !strings.Contains(err.Error()+" "+errs.Hint(err), c.wantSaid) {
				t.Errorf("error %v (hint %q) does not mention %q", err, errs.Hint(err), c.wantSaid)
			}
		})
	}
}

func TestSelectorViewerIsResolvedClientSide(t *testing.T) {
	s, err := ParseSelector("kind:mr reviewer:@me author:@me")
	if err != nil {
		t.Fatal(err)
	}
	if !s.NeedsViewer() {
		t.Fatal("a selector using @me does not report needing a viewer")
	}
	if err := s.ResolveViewer("codyconfer"); err != nil {
		t.Fatal(err)
	}
	q := s.Query()
	if q.Get("reviewer_username") != "codyconfer" || q.Get("author_username") != "codyconfer" {
		t.Errorf("query = %v, want both usernames substituted", q)
	}
	if strings.Contains(q.Encode(), "%40me") || strings.Contains(q.Encode(), "@me") {
		t.Errorf("query %q still carries @me; GitLab has no server-side alias, so it would match a "+
			"user literally named @me and return an empty list with a 200", q.Encode())
	}
}

func TestSelectorViewerFailureNamesTheSetting(t *testing.T) {
	s, err := ParseSelector("kind:mr reviewer:@me")
	if err != nil {
		t.Fatal(err)
	}
	err = s.ResolveViewer("")
	if err == nil {
		t.Fatal("an unresolvable @me was allowed through; the request would silently return nothing")
	}
	if !strings.Contains(errs.Hint(err), "gitlab.viewer") {
		t.Errorf("hint = %q, want it to name gitlab.viewer", errs.Hint(err))
	}
}

func TestSelectorScopeAssignedNeedsNoViewer(t *testing.T) {
	s, err := ParseSelector("kind:mr scope:assigned state:opened")
	if err != nil {
		t.Fatal(err)
	}
	if s.NeedsViewer() {
		t.Error("scope:assigned asked for a viewer; it resolves server-side and should save the " +
			"/user round trip")
	}
}

func TestSelectorRejectsViewerOnANonUsernameTerm(t *testing.T) {
	if _, err := ParseSelector("kind:mr label:@me"); err == nil {
		t.Fatal("@me was accepted as a label")
	}
}

func TestSelectorTermsCoverEveryKey(t *testing.T) {
	listed := map[string]bool{}
	for _, term := range SelectorTerms() {
		key, _, _ := strings.Cut(term, ":")
		listed[key] = true
	}
	for key := range terms {
		if !listed[key] {
			t.Errorf("term %q is parseable but absent from SelectorTerms, so completion and the query "+
				"builder never offer it", key)
		}
	}
	if !listed["kind"] {
		t.Error("SelectorTerms omits kind")
	}
}
