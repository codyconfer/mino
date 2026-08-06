package gitea

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/signals"
)

func TestParseRefAcceptsWebAndAPIURLs(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    Ref
		wantErr bool
	}{
		{name: "gitea pulls path", in: "https://git.example.com/acme/tools/pulls/12", want: Ref{"acme", "tools", 12, true}},
		{name: "github style pull path", in: "https://git.example.com/acme/tools/pull/12", want: Ref{"acme", "tools", 12, true}},
		{name: "issues path", in: "https://git.example.com/acme/tools/issues/9", want: Ref{"acme", "tools", 9, false}},
		{name: "subpath install", in: "https://example.com/gitea/acme/tools/issues/9", want: Ref{"acme", "tools", 9, false}},
		{name: "api url", in: "https://git.example.com/api/v1/repos/acme/tools/issues/9", want: Ref{"acme", "tools", 9, false}},
		{name: "empty", in: "", wantErr: true},
		{name: "no number", in: "https://git.example.com/acme/tools/pulls", wantErr: true},
		{name: "zero number", in: "https://git.example.com/acme/tools/pulls/0", wantErr: true},
		{name: "not an issue", in: "https://git.example.com/acme/tools/releases/1", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseRef(c.in)
			if (err != nil) != c.wantErr {
				t.Fatalf("ParseRef(%q) err = %v, wantErr %v", c.in, err, c.wantErr)
			}
			if c.wantErr {
				if errs.KindOf(err) != errs.KindUsage {
					t.Errorf("kind = %v, want %v", errs.KindOf(err), errs.KindUsage)
				}
				return
			}
			if got != c.want {
				t.Errorf("ParseRef(%q) = %+v, want %+v", c.in, got, c.want)
			}
		})
	}
}

const issueDetailFixture = `{"number":9,"title":"cannot log in","body":"500 on submit","state":"open","comments":45,
 "html_url":"https://git.example.com/acme/tools/issues/9","created_at":"2026-07-01T10:00:00Z",
 "updated_at":"2026-07-20T15:04:05Z","user":{"login":"carol"},"labels":[{"name":"bug"}],
 "assignees":[{"login":"bob"}],"milestone":{"title":"v2"}}`

const prDetailFixture = `{"number":12,"title":"fix the flaky test","body":"it flakes","state":"open","comments":0,
 "html_url":"https://git.example.com/acme/tools/pulls/12","created_at":"2026-07-02T10:00:00Z",
 "updated_at":"2026-07-20T15:04:05Z","user":{"login":"alice"},"pull_request":{"merged":false,"draft":false}}`

func TestDetailBoundsItsCalls(t *testing.T) {
	t.Run("issue with comments", func(t *testing.T) {
		b := &fakeBackend{issue: []byte(issueDetailFixture), comments: []byte(`[{"body":"still broken","created_at":"2026-07-20T15:00:00Z","user":{"login":"dave"}}]`)}
		s := newSignal(t, []string{"type:issues"}, b).(*Signal)

		d, err := s.Detail(context.Background(), signals.Item{URL: "https://git.example.com/acme/tools/issues/9"})
		if err != nil {
			t.Fatal(err)
		}
		if b.count("issue") != 1 || b.count("comments") != 1 || b.count("pull") != 0 || b.count("reviews") != 0 {
			t.Errorf("calls = %v, want one issue and one comments read for an issue", b.calls)
		}
		if b.commentsPage != 3 || b.commentsLimit != 20 {
			t.Errorf("comments page/limit = %d/%d, want 3/20: 45 comments means the newest page is the third", b.commentsPage, b.commentsLimit)
		}
		if d.Kind != "issue" || d.Title != "cannot log in" {
			t.Errorf("detail = %+v, want the issue", d)
		}
		if !strings.Contains(sectionTitles(d), "comments (latest 1 of 45)") {
			t.Errorf("sections = %q, want the comment count to say how many were fetched", sectionTitles(d))
		}
	})

	t.Run("issue with no comments", func(t *testing.T) {
		b := &fakeBackend{issue: []byte(strings.Replace(issueDetailFixture, `"comments":45`, `"comments":0`, 1))}
		s := newSignal(t, []string{"type:issues"}, b).(*Signal)

		if _, err := s.Detail(context.Background(), signals.Item{URL: "https://git.example.com/acme/tools/issues/9"}); err != nil {
			t.Fatal(err)
		}
		if b.count("comments") != 0 {
			t.Errorf("calls = %v, want no comments request when the issue reports none", b.calls)
		}
	})

	t.Run("pull request", func(t *testing.T) {
		b := &fakeBackend{
			issue:   []byte(prDetailFixture),
			pull:    []byte(`{"merged":false,"mergeable":true,"draft":false,"additions":12,"deletions":3,"changed_files":2,"requested_reviewers":[{"login":"bob"}]}`),
			reviews: []byte(`[{"state":"APPROVED","submitted_at":"2026-07-20T12:00:00Z","user":{"login":"bob"}}]`),
		}
		s := newSignal(t, []string{"type:pulls"}, b).(*Signal)

		d, err := s.Detail(context.Background(), signals.Item{URL: "https://git.example.com/acme/tools/pulls/12"})
		if err != nil {
			t.Fatal(err)
		}
		if b.count("issue") != 1 || b.count("pull") != 1 || b.count("reviews") != 1 {
			t.Errorf("calls = %v, want issue, pull and reviews", b.calls)
		}
		if !strings.Contains(sectionTitles(d), "reviews") {
			t.Errorf("sections = %q, want a reviews section", sectionTitles(d))
		}
		if !hasChip(d, "approved") {
			t.Errorf("chips = %v, want the derived review decision", d.Chips)
		}
		if !hasRow(d, "diff", "+12 −3 across 2 files") {
			t.Errorf("rows = %v, want a diff summary", d.Rows)
		}
	})
}

func TestDetailChipsCoverGiteaStates(t *testing.T) {
	cases := []struct {
		name     string
		pull     string
		reviews  string
		wantChip string
		wantSev  glyph.Severity
	}{
		{
			name: "changes requested outranks an approval",
			pull: `{"mergeable":true,"requested_reviewers":[]}`,
			reviews: `[{"state":"APPROVED","submitted_at":"2026-07-20T10:00:00Z","user":{"login":"bob"}},
				{"state":"REQUEST_CHANGES","submitted_at":"2026-07-20T11:00:00Z","user":{"login":"carol"}}]`,
			wantChip: "changes requested", wantSev: glyph.SeverityWarning,
		},
		{
			name:     "github spelling is understood too",
			pull:     `{"mergeable":true}`,
			reviews:  `[{"state":"CHANGES_REQUESTED","submitted_at":"2026-07-20T11:00:00Z","user":{"login":"carol"}}]`,
			wantChip: "changes requested", wantSev: glyph.SeverityWarning,
		},
		{
			name:     "only the latest review per user counts",
			pull:     `{"mergeable":true}`,
			reviews:  `[{"state":"REQUEST_CHANGES","submitted_at":"2026-07-20T10:00:00Z","user":{"login":"bob"}},{"state":"APPROVED","submitted_at":"2026-07-20T12:00:00Z","user":{"login":"bob"}}]`,
			wantChip: "approved", wantSev: glyph.SeverityPositive,
		},
		{
			name:     "comment reviews do not decide",
			pull:     `{"mergeable":true,"requested_reviewers":[{"login":"bob"}]}`,
			reviews:  `[{"state":"COMMENT","submitted_at":"2026-07-20T10:00:00Z","user":{"login":"bob"}}]`,
			wantChip: "review required", wantSev: glyph.SeverityNeutral,
		},
		{
			name:     "conflicts when the instance says unmergeable",
			pull:     `{"merged":false,"mergeable":false}`,
			reviews:  `[]`,
			wantChip: "conflicts", wantSev: glyph.SeverityWarning,
		},
		{
			name:     "no mergeable field means no claim",
			pull:     `{"merged":false}`,
			reviews:  `[]`,
			wantChip: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := &fakeBackend{issue: []byte(prDetailFixture), pull: []byte(c.pull), reviews: []byte(c.reviews)}
			s := newSignal(t, []string{"type:pulls"}, b).(*Signal)
			d, err := s.Detail(context.Background(), signals.Item{URL: "https://git.example.com/acme/tools/pulls/12"})
			if err != nil {
				t.Fatal(err)
			}
			if c.wantChip == "" {
				if hasChip(d, "conflicts") {
					t.Errorf("chips = %v, want no conflicts claim when mergeable is absent", d.Chips)
				}
				return
			}
			for _, chip := range d.Chips {
				if chip.Label == c.wantChip {
					if chip.Sev != c.wantSev {
						t.Errorf("chip %q severity = %v, want %v", chip.Label, chip.Sev, c.wantSev)
					}
					return
				}
			}
			t.Errorf("chips = %v, want %q", d.Chips, c.wantChip)
		})
	}
}

func TestDetailStateSeverity(t *testing.T) {
	cases := []struct {
		state string
		isPR  bool
		want  glyph.Severity
	}{
		{"merged", true, glyph.SeverityPositive},
		{"closed", true, glyph.SeverityNegative},
		{"closed", false, glyph.SeverityPositive},
		{"open", true, glyph.SeverityNeutral},
	}
	for _, c := range cases {
		if got := stateSeverity(c.state, c.isPR); got != c.want {
			t.Errorf("stateSeverity(%q, pr=%v) = %v, want %v", c.state, c.isPR, got, c.want)
		}
	}
}

type memCache struct {
	entries map[string]string
	reads   int
	writes  int
}

func (m *memCache) Get(_ context.Context, ns, key string) (string, bool) {
	m.reads++
	v, ok := m.entries[ns+"/"+key]
	return v, ok
}

func (m *memCache) Put(_ context.Context, ns, key, value string, _ time.Time) {
	m.writes++
	if m.entries == nil {
		m.entries = map[string]string{}
	}
	m.entries[ns+"/"+key] = value
}

func TestDetailCacheAvoidsRefetching(t *testing.T) {
	b := &fakeBackend{issue: []byte(strings.Replace(issueDetailFixture, `"comments":45`, `"comments":0`, 1))}
	c := &memCache{}
	s := newSignal(t, []string{"type:issues"}, b, WithDetailCache(c, CachePolicy{Read: true, Write: true, TTL: time.Minute})).(*Signal)
	item := signals.Item{URL: "https://git.example.com/acme/tools/issues/9"}

	if _, err := s.Detail(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Detail(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if b.count("issue") != 1 {
		t.Errorf("issue was fetched %d times, want one fetch and one cache hit", b.count("issue"))
	}

	c.entries[detailCacheNS+"/acme/tools#9"] = "{not json"
	if _, err := s.Detail(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if b.count("issue") != 2 {
		t.Error("a corrupt cache entry was not discarded and refetched")
	}
}

func TestDetailCacheIsDisabledWithoutATTL(t *testing.T) {
	b := &fakeBackend{issue: []byte(strings.Replace(issueDetailFixture, `"comments":45`, `"comments":0`, 1))}
	c := &memCache{}
	s := newSignal(t, []string{"type:issues"}, b, WithDetailCache(c, CachePolicy{Read: true, Write: true})).(*Signal)
	item := signals.Item{URL: "https://git.example.com/acme/tools/issues/9"}

	for range 2 {
		if _, err := s.Detail(context.Background(), item); err != nil {
			t.Fatal(err)
		}
	}
	if c.reads != 0 || c.writes != 0 {
		t.Errorf("cache reads/writes = %d/%d with no TTL, want none", c.reads, c.writes)
	}
	if b.count("issue") != 2 {
		t.Errorf("issue fetched %d times, want one per call", b.count("issue"))
	}
}

func TestDetailReportsAMissingItem(t *testing.T) {
	b := &fakeBackend{issue: []byte(`{}`)}
	s := newSignal(t, []string{"type:issues"}, b).(*Signal)

	_, err := s.Detail(context.Background(), signals.Item{URL: "https://git.example.com/acme/tools/issues/9"})
	if errs.KindOf(err) != errs.KindUsage {
		t.Errorf("err = %v, want a usage error naming the missing item", err)
	}
}

func sectionTitles(d signals.ItemDetail) string {
	var out []string
	for _, s := range d.Sections {
		out = append(out, s.Title)
	}
	return strings.Join(out, "|")
}

func hasChip(d signals.ItemDetail, label string) bool {
	for _, c := range d.Chips {
		if c.Label == label {
			return true
		}
	}
	return false
}

func hasRow(d signals.ItemDetail, key, value string) bool {
	for _, r := range d.Rows {
		if r[0] == key && r[1] == value {
			return true
		}
	}
	return false
}
