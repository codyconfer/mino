package gitea

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/errs"
)

func mustParse(t *testing.T, raw string) Query {
	t.Helper()
	q, err := ParseQuery(raw)
	if err != nil {
		t.Fatalf("ParseQuery(%q): %v", raw, err)
	}
	return q
}

func TestParseQueryBuildsGiteaParams(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		wantPath string
		want     url.Values
	}{
		{
			name: "defaults to open", raw: "", wantPath: searchPath,
			want: url.Values{"state": {"open"}, "limit": {"30"}, "page": {"1"}},
		},
		{
			name: "review requests", raw: "type:pulls state:open review_requested:@me", wantPath: searchPath,
			want: url.Values{"state": {"open"}, "type": {"pulls"}, "review_requested": {"true"}, "limit": {"30"}, "page": {"1"}},
		},
		{
			name: "github shaped aliases", raw: "is:open is:pr review-requested:@me", wantPath: searchPath,
			want: url.Values{"state": {"open"}, "type": {"pulls"}, "review_requested": {"true"}, "limit": {"30"}, "page": {"1"}},
		},
		{
			name: "author me becomes created", raw: "type:pr author:@me", wantPath: searchPath,
			want: url.Values{"state": {"open"}, "type": {"pulls"}, "created": {"true"}, "limit": {"30"}, "page": {"1"}},
		},
		{
			name: "labels and milestone", raw: `repo:acme/tools type:issues labels:bug,regression milestone:"v2 ga"`,
			wantPath: "/repos/acme/tools/issues",
			want: url.Values{"state": {"open"}, "type": {"issues"}, "labels": {"bug,regression"},
				"milestones": {"v2 ga"}, "limit": {"30"}, "page": {"1"}},
		},
		{
			name: "owner and text", raw: `owner:acme state:all q:"flaky test" upload`, wantPath: searchPath,
			want: url.Values{"state": {"all"}, "owner": {"acme"}, "q": {"flaky test upload"}, "limit": {"30"}, "page": {"1"}},
		},
		{
			name: "per repo actor takes a login", raw: "repo:acme/tools created:alice", wantPath: "/repos/acme/tools/issues",
			want: url.Values{"state": {"open"}, "created_by": {"alice"}, "limit": {"30"}, "page": {"1"}},
		},
		{
			name: "type all is omitted", raw: "type:all state:closed", wantPath: searchPath,
			want: url.Values{"state": {"closed"}, "limit": {"30"}, "page": {"1"}},
		},
		{
			name: "false actor is dropped", raw: "reviewed:false", wantPath: searchPath,
			want: url.Values{"state": {"open"}, "limit": {"30"}, "page": {"1"}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q := mustParse(t, c.raw)
			if got := q.Path(); got != c.wantPath {
				t.Errorf("Path() = %q, want %q", got, c.wantPath)
			}
			if got := q.Values(30, 1); got.Encode() != c.want.Encode() {
				t.Errorf("Values() = %q, want %q", got.Encode(), c.want.Encode())
			}
		})
	}
}

func TestParseQueryResolvesRelativeWindows(t *testing.T) {
	q := mustParse(t, "since:7d")
	since, err := time.Parse(time.RFC3339, q.Since)
	if err != nil {
		t.Fatalf("since = %q, want RFC3339: %v", q.Since, err)
	}
	want := time.Now().Add(-7 * 24 * time.Hour)
	if diff := since.Sub(want); diff > time.Minute || diff < -time.Minute {
		t.Errorf("since = %s, want about %s", since, want)
	}

	q = mustParse(t, "before:2026-01-02T03:04:05Z")
	if q.Before != "2026-01-02T03:04:05Z" {
		t.Errorf("before = %q, want the timestamp passed through", q.Before)
	}
}

func TestParseQueryRejectsWhatGiteaCannotExpress(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		wantHint string
	}{
		{"unknown qualifier", "stat:open", "supported qualifiers"},
		{"valueless qualifier", "labels:", ""},
		{"bad type", "type:pullrequests", "supported values"},
		{"bad state", "state:merged", "supported values"},
		{"bad is", "is:draft", "supported values"},
		{"cross repo login", "created:alice", "repo:owner/name"},
		{"review requested login", "review_requested:alice", "no per-user form"},
		{"two repos", "repo:a/b repo:c/d", "one repository or every repository"},
		{"review requested per repo", "repo:a/b review_requested:@me", "drop repo:a/b"},
		{"owner with repo", "repo:a/b owner:acme", "already names the owner"},
		{"bad repo", "repo:acme", ""},
		{"bad window", "since:soon", "RFC3339"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseQuery(c.raw)
			if err == nil {
				t.Fatalf("ParseQuery(%q) succeeded; an unsupported qualifier must be a config error rather than a silently ignored term", c.raw)
			}
			if errs.KindOf(err) != errs.KindConfig {
				t.Errorf("kind = %v, want %v", errs.KindOf(err), errs.KindConfig)
			}
			if c.wantHint != "" && !strings.Contains(errs.Hint(err)+err.Error(), c.wantHint) {
				t.Errorf("error %q / hint %q is missing %q", err, errs.Hint(err), c.wantHint)
			}
		})
	}
}

func TestNeedsViewerLoginOnlyWhereGiteaWantsALogin(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"type:pulls review_requested:@me", false},
		{"type:pulls created:@me", false},
		{"assigned:@me mentioned:@me reviewed:@me", false},
		{"owner:@me", true},
		{"repo:acme/tools created:@me", true},
		{"repo:acme/tools created:alice", false},
	}
	for _, c := range cases {
		t.Run(c.raw, func(t *testing.T) {
			q := mustParse(t, c.raw)
			if got := q.NeedsViewerLogin(); got != c.want {
				t.Errorf("NeedsViewerLogin() = %v, want %v; gitea resolves the cross-repo actor filters against the token itself", got, c.want)
			}
		})
	}
}

func TestWithViewerFillsOnlyTheLoginPositions(t *testing.T) {
	q := mustParse(t, "repo:acme/tools created:@me").WithViewer("alice")
	if got := q.Values(30, 1).Get("created_by"); got != "alice" {
		t.Errorf("created_by = %q, want alice", got)
	}

	q = mustParse(t, "owner:@me review_requested:@me").WithViewer("alice")
	v := q.Values(30, 1)
	if v.Get("owner") != "alice" {
		t.Errorf("owner = %q, want alice", v.Get("owner"))
	}
	if v.Get("review_requested") != "true" {
		t.Errorf("review_requested = %q, want true: the viewer must not leak into a boolean filter", v.Get("review_requested"))
	}
}

func TestDefaultQueriesParse(t *testing.T) {
	for _, raw := range DefaultQueries() {
		if _, err := ParseQuery(raw); err != nil {
			t.Errorf("ParseQuery(%q): %v", raw, err)
		}
	}
}
