package gitlab

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/signals"
)

func TestParseRef(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want Ref
	}{
		{
			"merge request", "https://gitlab.com/acme/api/-/merge_requests/42",
			Ref{Project: "acme/api", Kind: KindMR, ID: 42},
		},
		{
			"four-deep subgroup", "https://gitlab.com/a/b/c/d/-/issues/7",
			Ref{Project: "a/b/c/d", Kind: KindIssue, ID: 7},
		},
		{
			"pipeline", "https://gitlab.com/acme/api/-/pipelines/99",
			Ref{Project: "acme/api", Kind: KindPipeline, ID: 99},
		},
		{
			"trailing segment", "https://gitlab.com/acme/api/-/merge_requests/42/diffs",
			Ref{Project: "acme/api", Kind: KindMR, ID: 42},
		},
		{
			"note fragment", "https://gitlab.com/acme/api/-/merge_requests/42#note_881",
			Ref{Project: "acme/api", Kind: KindMR, ID: 42},
		},
		{
			"trailing slash", "https://gitlab.com/acme/api/-/issues/7/",
			Ref{Project: "acme/api", Kind: KindIssue, ID: 7},
		},
		{
			"self-managed host", "https://gitlab.example.com/acme/api/-/merge_requests/1",
			Ref{Project: "acme/api", Kind: KindMR, ID: 1},
		},
		{
			"legacy pre-slash-dash form", "https://gitlab.com/acme/api/merge_requests/42",
			Ref{Project: "acme/api", Kind: KindMR, ID: 42},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseRef(c.url)
			if err != nil {
				t.Fatalf("ParseRef(%q): %v", c.url, err)
			}
			if got != c.want {
				t.Errorf("ParseRef(%q) = %+v, want %+v", c.url, got, c.want)
			}
		})
	}
}

func TestParseRefRejections(t *testing.T) {
	cases := []struct {
		name     string
		url      string
		wantSaid string
	}{
		{"empty", "", "URL"},
		{"job", "https://gitlab.com/acme/api/-/jobs/5", "job"},
		{"zero iid", "https://gitlab.com/acme/api/-/merge_requests/0", "number"},
		{"non numeric", "https://gitlab.com/acme/api/-/merge_requests/abc", "number"},
		{"no number", "https://gitlab.com/acme/api/-/merge_requests", "names no"},
		{"no project", "https://gitlab.com/-/merge_requests/1", "project"},
		{"unrelated page", "https://gitlab.com/acme/api/-/settings/ci_cd", "not a merge request"},
		{"github url", "https://github.com/owner/repo/pull/1", "not a merge request"},
		{"no path", "https://gitlab.com", "path"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseRef(c.url)
			if err == nil {
				t.Fatalf("ParseRef(%q) was accepted", c.url)
			}
			if errs.KindOf(err) != errs.KindUsage {
				t.Errorf("kind = %v, want usage", errs.KindOf(err))
			}
			if !strings.Contains(err.Error(), c.wantSaid) {
				t.Errorf("error = %v, want it to mention %q", err, c.wantSaid)
			}
		})
	}
}

func TestRefStringIsSurfaceDistinct(t *testing.T) {
	mr := Ref{Project: "acme/api", Kind: KindMR, ID: 42}
	issue := Ref{Project: "acme/api", Kind: KindIssue, ID: 42}
	pipe := Ref{Project: "acme/api", Kind: KindPipeline, ID: 42}

	keys := map[string]bool{mr.String(): true, issue.String(): true, pipe.String(): true}
	if len(keys) != 3 {
		t.Errorf("cache keys collide across surfaces: %v; MR !42 and issue #42 would share a cache "+
			"entry in the same project", keys)
	}
}

func TestRefPath(t *testing.T) {
	ref := Ref{Project: "acme/platform/api", Kind: KindMR, ID: 42}
	if got := ref.path(""); got != "projects/acme%2Fplatform%2Fapi/merge_requests/42" {
		t.Errorf("path = %q", got)
	}
	if got := ref.path("approvals"); got != "projects/acme%2Fplatform%2Fapi/merge_requests/42/approvals" {
		t.Errorf("suffixed path = %q", got)
	}
}

type memCache struct {
	mu     sync.Mutex
	values map[string]string
	reads  int
	writes int
}

func newMemCache() *memCache { return &memCache{values: map[string]string{}} }

func (c *memCache) Get(_ context.Context, ns, key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reads++
	v, ok := c.values[ns+"|"+key]
	return v, ok
}

func (c *memCache) Put(_ context.Context, ns, key, value string, _ time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writes++
	c.values[ns+"|"+key] = value
}

const mrDetailFixture = `{
  "id": 900, "iid": 42, "project_id": 7,
  "title": "Drain the payments queue", "description": "the body",
  "state": "opened", "draft": false,
  "web_url": "https://gitlab.com/acme/api/-/merge_requests/42",
  "created_at": "2026-08-01T10:00:00Z", "updated_at": "2026-08-04T10:00:00Z",
  "source_branch": "fix/queue", "target_branch": "main",
  "labels": ["backend", "urgent"],
  "detailed_merge_status": "ci_must_pass", "changes_count": "12",
  "author": {"username": "codyconfer"},
  "assignees": [{"username": "codyconfer"}],
  "reviewers": [{"username": "reviewer1"}],
  "milestone": {"title": "24.9"}
}`

const approvalsFixture = `{
  "approvals_required": 2, "approvals_left": 1,
  "approved_by": [{"user": {"username": "reviewer1"}}]
}`

const headPipelineFixture = `[
  {"id": 90210, "iid": 12, "status": "running", "ref": "fix/queue",
   "web_url": "https://gitlab.com/acme/api/-/pipelines/90210"}
]`

const jobsFixture = `[
  {"id": 1, "name": "unit", "stage": "test", "status": "success", "duration": 72.4},
  {"id": 2, "name": "e2e", "stage": "test", "status": "running"},
  {"id": 3, "name": "deploy", "stage": "deploy", "status": "manual"}
]`

const diffsFixture = `[
  {"new_path": "api/queue.go", "old_path": "api/queue.go"},
  {"new_path": "api/new.go", "old_path": "api/new.go", "new_file": true},
  {"new_path": "api/gone.go", "old_path": "api/gone.go", "deleted_file": true}
]`

const notesFixture = `[
  {"id": 1, "body": "assigned to @codyconfer", "system": true,
   "created_at": "2026-08-01T11:00:00Z", "author": {"username": "codyconfer"}},
  {"id": 2, "body": "This looks good, one nit.", "system": false,
   "created_at": "2026-08-02T11:00:00Z", "author": {"username": "reviewer1"}},
  {"id": 3, "body": "pipeline failed", "system": false,
   "created_at": "2026-08-03T11:00:00Z", "author": {"username": "ci-bot", "bot": true}}
]`

func mrDetailRoutes() map[string]string {
	return map[string]string{
		"projects/acme%2Fapi/merge_requests/42":           mrDetailFixture,
		"projects/acme%2Fapi/merge_requests/42/approvals": approvalsFixture,
		"projects/acme%2Fapi/merge_requests/42/pipelines": headPipelineFixture,
		"projects/acme%2Fapi/merge_requests/42/diffs":     diffsFixture,
		"projects/acme%2Fapi/merge_requests/42/notes":     notesFixture,
		"projects/acme%2Fapi/pipelines/90210/jobs":        jobsFixture,
	}
}

func mrItem() signals.Item {
	return signals.Item{Kind: "mr", URL: "https://gitlab.com/acme/api/-/merge_requests/42"}
}

func chipLabels(d signals.ItemDetail) []string {
	out := make([]string, 0, len(d.Chips))
	for _, c := range d.Chips {
		out = append(out, c.Label)
	}
	return out
}

func rowValue(d signals.ItemDetail, key string) string {
	for _, r := range d.Rows {
		if r[0] == key {
			return r[1]
		}
	}
	return ""
}

func section(d signals.ItemDetail, prefix string) (signals.DetailSection, bool) {
	for _, s := range d.Sections {
		if strings.HasPrefix(s.Title, prefix) {
			return s, true
		}
	}
	return signals.DetailSection{}, false
}

func TestMergeRequestDetail(t *testing.T) {
	freezeClock(t, "2026-08-05T12:00:00Z")
	rec := newRecorder(mrDetailRoutes())
	s := newSignal(t, rec, []string{"kind:mr scope:assigned"})

	d, err := s.Detail(context.Background(), mrItem())
	if err != nil {
		t.Fatal(err)
	}

	if d.Kind != "mr" || d.Title != "Drain the payments queue" || d.Body != "the body" {
		t.Errorf("detail head = %+v", d)
	}
	labels := chipLabels(d)
	for _, want := range []string{"opened", "ci must pass", "pipeline running"} {
		if !containsString(labels, want) {
			t.Errorf("chips = %v, want %q; detailed_merge_status is GitLab's clearest \"why can't I "+
				"merge\" signal and has no GitHub equivalent", labels, want)
		}
	}

	for key, want := range map[string]string{
		"project":   "acme/api !42",
		"author":    "@codyconfer",
		"branches":  "fix/queue -> main",
		"labels":    "backend · urgent",
		"reviewers": "@reviewer1",
		"approvals": "1 of 2",
		"milestone": "24.9",
		"diff":      "12 file(s) changed",
	} {
		if got := rowValue(d, key); got != want {
			t.Errorf("row %q = %q, want %q", key, got, want)
		}
	}

	pipe, ok := section(d, "pipeline")
	if !ok {
		t.Fatal("no pipeline section")
	}
	if pipe.Meta["in_progress"] != "true" {
		t.Errorf("pipeline section meta = %v, want in_progress while a job is running; the deck uses "+
			"that key to keep refreshing", pipe.Meta)
	}
	if d.Meta["in_progress"] != "true" {
		t.Errorf("detail meta = %v, want in_progress promoted", d.Meta)
	}
	if len(pipe.Rows) != 3 || pipe.Rows[0][0] != "[test] unit" {
		t.Errorf("pipeline rows = %v, want jobs labelled by stage", pipe.Rows)
	}
	if !strings.Contains(pipe.Rows[0][1], "1m12s") {
		t.Errorf("job value = %q, want the duration", pipe.Rows[0][1])
	}

	if files, ok := section(d, "files"); !ok {
		t.Error("no files section")
	} else if len(files.Rows) != 3 || files.Rows[1][1] != "added" || files.Rows[2][1] != "deleted" {
		t.Errorf("files rows = %v", files.Rows)
	}

	if appr, ok := section(d, "approvals"); !ok {
		t.Error("no approvals section")
	} else if len(appr.Lines) != 1 || !strings.Contains(appr.Lines[0], "1 approval") {
		t.Errorf("approvals section = %+v", appr)
	}
}

func TestMergeRequestDetailDropsSystemNotesAndMarksBots(t *testing.T) {
	freezeClock(t, "2026-08-05T12:00:00Z")
	rec := newRecorder(mrDetailRoutes())
	s := newSignal(t, rec, []string{"kind:mr scope:assigned"})

	d, err := s.Detail(context.Background(), mrItem())
	if err != nil {
		t.Fatal(err)
	}
	notes, ok := section(d, "comments")
	if !ok {
		t.Fatal("no comments section")
	}
	if strings.Contains(notes.Body, "assigned to") {
		t.Error("a system note reached the thread; GitLab records assignment and commit pushes as " +
			"notes, so an unfiltered thread is mostly bookkeeping")
	}
	if !strings.Contains(notes.Body, "one nit") {
		t.Error("the human comment is missing")
	}
	if !strings.Contains(notes.Body, "·bot") {
		t.Error("a bot author was not marked")
	}
}

func TestMergeRequestDetailDegradesWhenApprovalsAreForbidden(t *testing.T) {
	freezeClock(t, "2026-08-05T12:00:00Z")
	rec := newRecorder(mrDetailRoutes())
	rec.status["projects/acme%2Fapi/merge_requests/42/approvals"] = http.StatusForbidden
	s := newSignal(t, rec, []string{"kind:mr scope:assigned"})

	d, err := s.Detail(context.Background(), mrItem())
	if err != nil {
		t.Fatalf("a 403 on a best-effort endpoint failed the whole detail: %v", err)
	}
	if _, ok := section(d, "approvals"); ok {
		t.Error("an approvals section was rendered from a forbidden response")
	}
	if _, ok := section(d, "files"); !ok {
		t.Error("the rest of the detail was lost along with approvals")
	}
	if rowValue(d, "project") == "" {
		t.Error("the core MR fields are missing")
	}
}

func TestMergeRequestDetailFailsWhenTheMRItselfIsUnreadable(t *testing.T) {
	rec := newRecorder(mrDetailRoutes())
	rec.status["projects/acme%2Fapi/merge_requests/42"] = http.StatusUnauthorized
	s := newSignal(t, rec, []string{"kind:mr scope:assigned"})

	if _, err := s.Detail(context.Background(), mrItem()); err == nil {
		t.Fatal("an unreadable merge request produced a detail page anyway")
	}
}

func TestDetailCacheHitSkipsTheNetwork(t *testing.T) {
	freezeClock(t, "2026-08-05T12:00:00Z")
	rec := newRecorder(mrDetailRoutes())
	cache := newMemCache()
	s := newSignal(t, rec, []string{"kind:mr scope:assigned"},
		WithDetailCache(cache, CachePolicy{Read: true, Write: true, TTL: time.Minute}))

	if _, err := s.Detail(context.Background(), mrItem()); err != nil {
		t.Fatal(err)
	}
	first := len(rec.seen())
	if cache.writes == 0 {
		t.Fatal("a miss did not write to the cache")
	}

	d, err := s.Detail(context.Background(), mrItem())
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.seen()) != first {
		t.Errorf("a cache hit still made %d requests", len(rec.seen())-first)
	}
	if d.Title != "Drain the payments queue" {
		t.Errorf("cached detail = %q", d.Title)
	}
}

func TestDetailCacheDiscardsAnUnreadableEntry(t *testing.T) {
	freezeClock(t, "2026-08-05T12:00:00Z")
	rec := newRecorder(mrDetailRoutes())
	cache := newMemCache()
	cache.values[detailCacheNS+"|acme/api!42"] = "not json"
	s := newSignal(t, rec, []string{"kind:mr scope:assigned"},
		WithDetailCache(cache, CachePolicy{Read: true, Write: true, TTL: time.Minute}))

	d, err := s.Detail(context.Background(), mrItem())
	if err != nil {
		t.Fatal(err)
	}
	if d.Title == "" {
		t.Error("a corrupt cache entry was served instead of refetching")
	}
}

func TestPipelineDetailNeverTouchesTheCache(t *testing.T) {
	freezeClock(t, "2026-08-05T12:00:00Z")
	rec := newRecorder(map[string]string{
		"projects/acme%2Fapi/pipelines/99": `{
		  "id": 99, "iid": 12, "status": "running", "source": "push", "ref": "main",
		  "sha": "abcdef0123456789aaaa", "duration": 90, "queued_duration": 5,
		  "user": {"username": "codyconfer"},
		  "created_at": "2026-08-05T11:00:00Z", "updated_at": "2026-08-05T11:30:00Z",
		  "web_url": "https://gitlab.com/acme/api/-/pipelines/99"
		}`,
		"projects/acme%2Fapi/pipelines/99/jobs": jobsFixture,
	})
	cache := newMemCache()
	s := newSignal(t, rec, []string{"kind:mr scope:assigned"},
		WithDetailCache(cache, CachePolicy{Read: true, Write: true, TTL: time.Minute}))

	it := signals.Item{Kind: "pipeline", URL: "https://gitlab.com/acme/api/-/pipelines/99"}
	d, err := s.Detail(context.Background(), it)
	if err != nil {
		t.Fatal(err)
	}
	if cache.reads != 0 || cache.writes != 0 {
		t.Errorf("pipeline detail touched the cache (%d reads, %d writes); a running pipeline's state "+
			"churns, so a cached one is worse than none", cache.reads, cache.writes)
	}

	if d.Kind != "pipeline" || d.Meta["state"] != "running" {
		t.Errorf("detail = %+v", d)
	}
	if d.Meta["in_progress"] != "true" {
		t.Errorf("meta = %v, want in_progress", d.Meta)
	}
	if got := rowValue(d, "commit"); got != "abcdef012345" {
		t.Errorf("commit = %q, want it truncated to 12", got)
	}
	if got := rowValue(d, "triggered by"); got != "@codyconfer" {
		t.Errorf("triggered by = %q", got)
	}
	if got := rowValue(d, "duration"); got != "1m35s" {
		t.Errorf("duration = %q, want run plus queued time", got)
	}
	if labels := chipLabels(d); !containsString(labels, "push") {
		t.Errorf("chips = %v, want the pipeline source", labels)
	}
}

func TestIssueDetail(t *testing.T) {
	freezeClock(t, "2026-08-05T12:00:00Z")
	rec := newRecorder(map[string]string{
		"projects/acme%2Fapi/issues/7": `{
		  "id": 500, "iid": 7, "title": "Flaky test", "description": "sometimes red",
		  "state": "opened", "confidential": true, "due_date": "2026-09-01", "weight": 3,
		  "labels": ["test"], "author": {"username": "codyconfer"},
		  "assignees": [{"username": "codyconfer"}], "milestone": {"title": "24.9"},
		  "created_at": "2026-08-01T10:00:00Z", "updated_at": "2026-08-02T10:00:00Z",
		  "web_url": "https://gitlab.com/acme/api/-/issues/7"
		}`,
		"projects/acme%2Fapi/issues/7/notes": notesFixture,
	})
	s := newSignal(t, rec, []string{"kind:issue scope:assigned"})

	it := signals.Item{Kind: "issue", URL: "https://gitlab.com/acme/api/-/issues/7"}
	d, err := s.Detail(context.Background(), it)
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind != "issue" || rowValue(d, "project") != "acme/api #7" {
		t.Errorf("detail = %+v", d)
	}
	if labels := chipLabels(d); !containsString(labels, "confidential") {
		t.Errorf("chips = %v, want the confidential marker", labels)
	}
	for key, want := range map[string]string{"due date": "2026-09-01", "weight": "3"} {
		if got := rowValue(d, key); got != want {
			t.Errorf("row %q = %q, want %q", key, got, want)
		}
	}
	if _, ok := section(d, "files"); ok {
		t.Error("an issue detail carries a files section")
	}
	if _, ok := section(d, "comments"); !ok {
		t.Error("an issue detail carries no comments")
	}
}

func TestClosedIssueChipIsPositive(t *testing.T) {
	if got := issueStateSeverity("closed"); got != glyph.SeverityPositive {
		t.Errorf("closed issue chip = %v, want positive", got)
	}
	if got := mrStateSeverity("merged"); got != glyph.SeverityPositive {
		t.Errorf("merged MR chip = %v, want positive", got)
	}
	if got := mrStateSeverity("closed"); got != glyph.SeverityNegative {
		t.Errorf("closed MR chip = %v, want negative", got)
	}
}

func TestMergeBlockerIsQuietWhenMergeable(t *testing.T) {
	for _, s := range []string{"", "mergeable", "checking", "unchecked", "not_open"} {
		if _, ok := mergeBlocker(s); ok {
			t.Errorf("mergeBlocker(%q) produced a warning chip for a non-blocking status", s)
		}
	}
	label, ok := mergeBlocker("discussions_not_resolved")
	if !ok || label != "discussions not resolved" {
		t.Errorf("mergeBlocker = %q/%v", label, ok)
	}
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
