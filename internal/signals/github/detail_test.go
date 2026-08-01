package github

import (
	"context"
	"errors"
	"strings"
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
		{"pull", "https://github.com/acme/tools/pull/412", Ref{"acme", "tools", 412, true}},
		{"issue", "https://github.com/acme/tools/issues/87", Ref{"acme", "tools", 87, false}},
		{"pull subpage", "https://github.com/acme/tools/pull/412/files", Ref{"acme", "tools", 412, true}},
		{"enterprise host", "https://ghe.corp.example/acme/tools/issues/9", Ref{"acme", "tools", 9, false}},
		{"trailing slash", "https://github.com/acme/tools/pull/1/", Ref{"acme", "tools", 1, true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseRef(c.url)
			if err != nil {
				t.Fatalf("ParseRef(%q) = %v", c.url, err)
			}
			if got != c.want {
				t.Errorf("ParseRef(%q) = %+v, want %+v", c.url, got, c.want)
			}
		})
	}
}

func TestParseRefRejectsJunk(t *testing.T) {
	for _, url := range []string{
		"",
		"https://github.com/acme/tools",
		"https://github.com/acme/tools/pull/abc",
		"https://github.com/acme/tools/pull",
		"https://example.com",
		"https://github.com/acme/tools/pull/0",
	} {
		if _, err := ParseRef(url); err == nil {
			t.Errorf("ParseRef(%q) = nil error, want a usage error", url)
		} else if errs.KindOf(err) != errs.KindUsage {
			t.Errorf("ParseRef(%q) kind = %v, want %v", url, errs.KindOf(err), errs.KindUsage)
		}
	}
}

func TestRefStringAndKind(t *testing.T) {
	r := Ref{Owner: "acme", Repo: "tools", Number: 412, IsPR: true}
	if got := r.String(); got != "acme/tools#412" {
		t.Errorf("String = %q", got)
	}
	if got := r.Slug(); got != "acme/tools" {
		t.Errorf("Slug = %q", got)
	}
	if got := r.Kind(); got != "pr" {
		t.Errorf("Kind = %q, want pr", got)
	}
	if got := (Ref{}).Kind(); got != "issue" {
		t.Errorf("Kind = %q, want issue", got)
	}
}

type fakeDetailBackend struct {
	body  string
	err   error
	calls int
	vars  map[string]any
}

func (f *fakeDetailBackend) SearchIssues(context.Context, string, int) ([]byte, error) {
	return nil, errors.New("unexpected search")
}

func (f *fakeDetailBackend) GraphQL(_ context.Context, query string, vars map[string]any) ([]byte, error) {
	if !strings.Contains(query, "issueOrPullRequest") {
		return nil, errors.New("unexpected query: " + query)
	}
	f.calls++
	f.vars = vars
	if f.err != nil {
		return nil, f.err
	}
	return []byte(f.body), nil
}

func cachingPolicy() CachePolicy {
	return CachePolicy{Read: true, Write: true, TTL: time.Minute}
}

func writeOnlyPolicy() CachePolicy {
	return CachePolicy{Write: true, TTL: time.Minute}
}

type memCache struct {
	data  map[string]string
	gets  int
	puts  int
	onPut func(expiry time.Time)
}

func newMemCache() *memCache { return &memCache{data: map[string]string{}} }

func (c *memCache) Get(_ context.Context, ns, key string) (string, bool) {
	c.gets++
	v, ok := c.data[ns+"/"+key]
	return v, ok
}

func (c *memCache) Put(_ context.Context, ns, key, value string, expiry time.Time) {
	c.puts++
	c.data[ns+"/"+key] = value
	if c.onPut != nil {
		c.onPut(expiry)
	}
}

func detailResponseJSON(node string) string {
	return `{"data":{"repository":{"issueOrPullRequest":` + node + `}}}`
}

const prDetailNode = `{
  "__typename":"PullRequest",
  "title":"fix backoff","url":"https://github.com/acme/tools/pull/412","body":"## Summary\nclamp it",
  "state":"OPEN","createdAt":"2026-07-01T10:00:00Z","updatedAt":"2026-07-20T10:00:00Z",
  "author":{"login":"cody"},"milestone":{"title":"v2"},
  "labels":{"nodes":[{"name":"bug"},{"name":"area/signals"}]},
  "assignees":{"nodes":[{"login":"cody"}]},
  "comments":{"totalCount":3,"nodes":[
    {"createdAt":"2026-07-19T10:00:00Z","body":"please clamp","author":{"login":"alice","__typename":"User"}},
    {"createdAt":"2026-07-20T09:00:00Z","body":"lint failed","author":{"login":"ci-bot","__typename":"Bot"}}]},
  "isDraft":true,"merged":false,"reviewDecision":"CHANGES_REQUESTED",
  "additions":42,"deletions":7,"changedFiles":3,
  "files":{"totalCount":5,"nodes":[{"path":"internal/poll.go","additions":30,"deletions":5}]},
  "reviewRequests":{"nodes":[{"requestedReviewer":{"login":"bob"}},{"requestedReviewer":{"name":"platform"}},{"requestedReviewer":null}]},
  "latestReviews":{"nodes":[{"state":"CHANGES_REQUESTED","submittedAt":"2026-07-19T11:00:00Z","author":{"login":"alice"}}]},
  "commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"FAILURE","contexts":{"nodes":[
    {"name":"build","conclusion":"SUCCESS","status":"COMPLETED"},
    {"name":"lint","conclusion":"FAILURE","status":"COMPLETED"},
    {"name":"lint","conclusion":"FAILURE","status":"COMPLETED"},
    {"context":"legacy/ci","state":"PENDING"}]}}}}]}}`

const issueDetailNode = `{
  "__typename":"Issue",
  "title":"branches lose push","url":"https://github.com/acme/tools/issues/87","body":"repro steps",
  "state":"CLOSED","createdAt":"2026-07-01T10:00:00Z","updatedAt":"2026-07-02T10:00:00Z",
  "author":{"login":"harald"},
  "labels":{"nodes":[{"name":"needs-triage"}]},
  "assignees":{"nodes":[]},
  "comments":{"totalCount":1,"nodes":[
    {"createdAt":"2026-07-02T10:00:00Z","body":"thanks","author":{"login":"cody","__typename":"User"}}]}}`

func prItem() signals.Item {
	return signals.Item{Kind: "pr", URL: "https://github.com/acme/tools/pull/412"}
}

func fetchPRDetail(t *testing.T) signals.ItemDetail {
	t.Helper()
	be := &fakeDetailBackend{body: detailResponseJSON(prDetailNode)}
	d, err := fetchDetail(context.Background(), be, nil, CachePolicy{}, prItem())
	if err != nil {
		t.Fatalf("fetchDetail: %v", err)
	}
	return d
}

func TestDetailMapsPullRequest(t *testing.T) {
	d := fetchPRDetail(t)

	if d.Kind != "pr" || d.Title != "fix backoff" {
		t.Errorf("kind/title = %q/%q", d.Kind, d.Title)
	}
	if !strings.Contains(d.Body, "clamp it") {
		t.Errorf("body = %q", d.Body)
	}

	wantChips := map[string]glyph.Severity{
		"open":              glyph.SeverityNeutral,
		"draft":             glyph.SeverityNeutral,
		"changes requested": glyph.SeverityWarning,
		"checks failure":    glyph.SeverityNegative,
	}
	if len(d.Chips) != len(wantChips) {
		t.Fatalf("chips = %+v, want %d", d.Chips, len(wantChips))
	}
	for _, c := range d.Chips {
		want, ok := wantChips[c.Label]
		if !ok {
			t.Errorf("unexpected chip %q", c.Label)
			continue
		}
		if c.Sev != want {
			t.Errorf("chip %q sev = %v, want %v", c.Label, c.Sev, want)
		}
	}

	rows := rowMap(d.Rows)
	for key, want := range map[string]string{
		"repo":      "acme/tools #412",
		"author":    "@cody",
		"labels":    "bug · area/signals",
		"assignees": "@cody",
		"reviewers": "@bob, platform",
		"milestone": "v2",
		"diff":      "+42 −7 across 3 files",
	} {
		if rows[key] != want {
			t.Errorf("row %q = %q, want %q", key, rows[key], want)
		}
	}
}

func TestDetailSectionsForPullRequest(t *testing.T) {
	d := fetchPRDetail(t)
	secs := sectionMap(d)

	checks, ok := secs["checks"]
	if !ok {
		t.Fatalf("no checks section in %+v", d.Sections)
	}
	if got := len(checks.Rows); got != 3 {
		t.Errorf("checks rows = %d, want 3 after dedupe: %+v", got, checks.Rows)
	}
	if got := rowMap(checks.Rows)["legacy/ci"]; got != "pending" {
		t.Errorf("StatusContext row = %q, want pending", got)
	}
	if checks.Meta["state"] != "failure" {
		t.Errorf("checks meta = %v", checks.Meta)
	}

	reviews, ok := secs["reviews"]
	if !ok {
		t.Fatal("no reviews section")
	}
	if got := reviews.Rows[0][0]; got != "@alice" {
		t.Errorf("review reviewer = %q", got)
	}
	if !strings.HasPrefix(reviews.Rows[0][1], "changes requested") {
		t.Errorf("review state = %q", reviews.Rows[0][1])
	}

	files, ok := secs["files"]
	if !ok {
		t.Fatal("no files section")
	}
	if got := rowMap(files.Rows)["internal/poll.go"]; got != "+30 −5" {
		t.Errorf("file row = %q", got)
	}
	if len(files.Lines) != 1 || files.Lines[0] != "+4 more" {
		t.Errorf("files overflow line = %v, want [+4 more]", files.Lines)
	}
}

func TestDetailCommentsMarkBots(t *testing.T) {
	d := fetchPRDetail(t)
	var body, title string
	for _, s := range d.Sections {
		if strings.HasPrefix(s.Title, "comments") {
			body, title = s.Body, s.Title
		}
	}
	if body == "" {
		t.Fatal("no comments section")
	}
	if !strings.Contains(body, "@ci-bot ·bot") {
		t.Errorf("bot comment not marked:\n%s", body)
	}
	if strings.Contains(body, "@alice ·bot") {
		t.Errorf("human comment marked as bot:\n%s", body)
	}
	if !strings.Contains(body, "please clamp") || !strings.Contains(body, "lint failed") {
		t.Errorf("comment bodies missing:\n%s", body)
	}
	if !strings.Contains(title, "latest 2 of 3") {
		t.Errorf("title = %q, want a truncation note", title)
	}
}

func TestDetailIssueOmitsPullRequestSections(t *testing.T) {
	be := &fakeDetailBackend{body: detailResponseJSON(issueDetailNode)}
	it := signals.Item{Kind: "issue", URL: "https://github.com/acme/tools/issues/87"}
	d, err := fetchDetail(context.Background(), be, nil, CachePolicy{}, it)
	if err != nil {
		t.Fatalf("fetchDetail: %v", err)
	}
	if d.Kind != "issue" {
		t.Errorf("kind = %q", d.Kind)
	}
	secs := sectionMap(d)
	for _, banned := range []string{"checks", "reviews", "files"} {
		if _, ok := secs[banned]; ok {
			t.Errorf("issue detail should omit the %q section", banned)
		}
	}
	if _, ok := secs["comments"]; !ok {
		t.Error("issue detail should keep comments")
	}
	if len(d.Chips) != 1 || d.Chips[0].Label != "closed" || d.Chips[0].Sev != glyph.SeverityPositive {
		t.Errorf("chips = %+v, want a positive closed chip", d.Chips)
	}
	if be.vars["n"] != 87 || be.vars["owner"] != "acme" || be.vars["repo"] != "tools" {
		t.Errorf("query vars = %+v", be.vars)
	}
}

func TestDetailMergedPullRequestIsPositive(t *testing.T) {
	node := strings.Replace(prDetailNode, `"state":"OPEN"`, `"state":"MERGED"`, 1)
	be := &fakeDetailBackend{body: detailResponseJSON(node)}
	d, err := fetchDetail(context.Background(), be, nil, CachePolicy{}, prItem())
	if err != nil {
		t.Fatalf("fetchDetail: %v", err)
	}
	if d.Chips[0].Label != "merged" || d.Chips[0].Sev != glyph.SeverityPositive {
		t.Errorf("first chip = %+v, want merged/positive", d.Chips[0])
	}
}

func TestDetailScopeErrorUsesRepoHint(t *testing.T) {
	be := &fakeDetailBackend{body: `{"errors":[{"type":"INSUFFICIENT_SCOPES","message":"needs more"}]}`}
	_, err := fetchDetail(context.Background(), be, nil, CachePolicy{}, prItem())
	if err == nil {
		t.Fatal("want an error")
	}
	if errs.KindOf(err) != errs.KindAuth {
		t.Errorf("kind = %v, want %v", errs.KindOf(err), errs.KindAuth)
	}
	hint := errs.Hint(err)
	if !strings.Contains(hint, "repo scope") {
		t.Errorf("hint = %q, want the repo scope hint", hint)
	}
	if strings.Contains(hint, "read:project") {
		t.Errorf("hint mentions read:project, which this query does not need: %q", hint)
	}
}

func TestDetailNotFoundIsUsageError(t *testing.T) {
	be := &fakeDetailBackend{body: `{"data":{"repository":{"issueOrPullRequest":null}},` +
		`"errors":[{"type":"NOT_FOUND","message":"Could not resolve to an issue or pull request"}]}`}
	_, err := fetchDetail(context.Background(), be, nil, CachePolicy{}, prItem())
	if err == nil {
		t.Fatal("want an error")
	}
	if errs.KindOf(err) != errs.KindUsage {
		t.Errorf("kind = %v, want %v", errs.KindOf(err), errs.KindUsage)
	}
}

func TestDetailMissingNodeWithoutErrors(t *testing.T) {
	be := &fakeDetailBackend{body: `{"data":{"repository":null}}`}
	_, err := fetchDetail(context.Background(), be, nil, CachePolicy{}, prItem())
	if err == nil || errs.KindOf(err) != errs.KindUsage {
		t.Errorf("err = %v, want a usage error", err)
	}
}

func TestDetailBadURLSkipsTheBackend(t *testing.T) {
	be := &fakeDetailBackend{body: detailResponseJSON(prDetailNode)}
	_, err := fetchDetail(context.Background(), be, nil, CachePolicy{}, signals.Item{URL: "nope"})
	if err == nil {
		t.Fatal("want an error")
	}
	if be.calls != 0 {
		t.Errorf("backend called %d times for an unparseable URL", be.calls)
	}
}

func TestDetailCacheRoundTrip(t *testing.T) {
	be := &fakeDetailBackend{body: detailResponseJSON(prDetailNode)}
	c := newMemCache()
	pol := cachingPolicy()

	first, err := fetchDetail(context.Background(), be, c, pol, prItem())
	if err != nil {
		t.Fatal(err)
	}
	if c.puts != 1 {
		t.Errorf("puts = %d, want 1", c.puts)
	}

	second, err := fetchDetail(context.Background(), be, c, pol, prItem())
	if err != nil {
		t.Fatal(err)
	}
	if be.calls != 1 {
		t.Errorf("backend called %d times, want 1 (second read should hit the cache)", be.calls)
	}
	if first.Title != second.Title || len(first.Sections) != len(second.Sections) {
		t.Errorf("cached detail differs from the live one")
	}
}

func TestDetailCachePolicies(t *testing.T) {
	warm := func(t *testing.T) *memCache {
		t.Helper()
		c := newMemCache()
		be := &fakeDetailBackend{body: detailResponseJSON(prDetailNode)}
		if _, err := fetchDetail(context.Background(), be, c, cachingPolicy(), prItem()); err != nil {
			t.Fatal(err)
		}
		return c
	}

	t.Run("refresh skips the read but still writes", func(t *testing.T) {
		c := warm(t)
		before := c.puts
		be := &fakeDetailBackend{body: detailResponseJSON(prDetailNode)}
		if _, err := fetchDetail(context.Background(), be, c, writeOnlyPolicy(), prItem()); err != nil {
			t.Fatal(err)
		}
		if be.calls != 1 {
			t.Errorf("backend calls = %d, want a live fetch", be.calls)
		}
		if c.puts != before+1 {
			t.Errorf("puts = %d, want %d", c.puts, before+1)
		}
	})

	t.Run("off neither reads nor writes", func(t *testing.T) {
		c := warm(t)
		gets, puts := c.gets, c.puts
		be := &fakeDetailBackend{body: detailResponseJSON(prDetailNode)}
		if _, err := fetchDetail(context.Background(), be, c, CachePolicy{}, prItem()); err != nil {
			t.Fatal(err)
		}
		if be.calls != 1 {
			t.Errorf("backend calls = %d, want a live fetch", be.calls)
		}
		if c.gets != gets || c.puts != puts {
			t.Errorf("cache touched: gets %d→%d, puts %d→%d", gets, c.gets, puts, c.puts)
		}
	})

	t.Run("unreadable entry falls back to the backend", func(t *testing.T) {
		c := newMemCache()
		c.data[detailCacheNS+"/acme/tools#412"] = "{not json"
		be := &fakeDetailBackend{body: detailResponseJSON(prDetailNode)}
		d, err := fetchDetail(context.Background(), be, c, cachingPolicy(), prItem())
		if err != nil {
			t.Fatal(err)
		}
		if be.calls != 1 || d.Title != "fix backoff" {
			t.Errorf("calls = %d, title = %q", be.calls, d.Title)
		}
	})
}

func TestDetailZeroTTLDisablesCaching(t *testing.T) {
	for _, pol := range []CachePolicy{
		{Read: true, Write: true, TTL: 0},
		{Read: true, Write: true, TTL: -time.Second},
	} {
		c := newMemCache()
		be := &fakeDetailBackend{body: detailResponseJSON(prDetailNode)}
		if _, err := fetchDetail(context.Background(), be, c, pol, prItem()); err != nil {
			t.Fatal(err)
		}
		if _, err := fetchDetail(context.Background(), be, c, pol, prItem()); err != nil {
			t.Fatal(err)
		}
		if c.gets != 0 || c.puts != 0 {
			t.Errorf("ttl %v: cache touched (gets=%d puts=%d), want none", pol.TTL, c.gets, c.puts)
		}
		if be.calls != 2 {
			t.Errorf("ttl %v: backend calls = %d, want 2 live fetches", pol.TTL, be.calls)
		}
	}
}

func TestDetailPutUsesThePolicyTTL(t *testing.T) {
	var expiry time.Time
	c := newMemCache()
	c.onPut = func(t time.Time) { expiry = t }
	be := &fakeDetailBackend{body: detailResponseJSON(prDetailNode)}
	pol := CachePolicy{Read: true, Write: true, TTL: 90 * time.Second}

	before := time.Now()
	if _, err := fetchDetail(context.Background(), be, c, pol, prItem()); err != nil {
		t.Fatal(err)
	}
	got := expiry.Sub(before)
	if got < 80*time.Second || got > 100*time.Second {
		t.Errorf("expiry is %v out, want about the policy TTL of %v", got, pol.TTL)
	}
}

func TestDetailErrorIsNotCached(t *testing.T) {
	c := newMemCache()
	be := &fakeDetailBackend{err: errors.New("network down")}
	if _, err := fetchDetail(context.Background(), be, c, cachingPolicy(), prItem()); err == nil {
		t.Fatal("want the backend error")
	}
	if c.puts != 0 {
		t.Errorf("puts = %d, want 0 on failure", c.puts)
	}
}

func TestSignalsImplementDetailer(t *testing.T) {
	var _ signals.Detailer = New(nil, &fakeDetailBackend{}, 10).(*Signal)
	var _ signals.Detailer = NewProject(ProjectSpec{Owner: "acme", Number: 1}, &fakeDetailBackend{}, 10, nil).(*projectSignal)
}

func TestWithDetailCacheIsApplied(t *testing.T) {
	c := newMemCache()
	pol := cachingPolicy()
	s := New(nil, &fakeDetailBackend{body: detailResponseJSON(prDetailNode)}, 10, WithDetailCache(c, pol)).(*Signal)
	if s.detail == nil || s.policy != pol {
		t.Fatalf("option not applied: detail=%v policy=%+v", s.detail != nil, s.policy)
	}
	if _, err := s.Detail(context.Background(), prItem()); err != nil {
		t.Fatal(err)
	}
	if c.puts != 1 {
		t.Errorf("puts = %d, want the signal to use its cache", c.puts)
	}
}

func rowMap(rows [][2]string) map[string]string {
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r[0]] = r[1]
	}
	return out
}

func sectionMap(d signals.ItemDetail) map[string]signals.DetailSection {
	out := make(map[string]signals.DetailSection, len(d.Sections))
	for _, s := range d.Sections {
		key := s.Title
		if i := strings.IndexByte(key, ' '); i > 0 {
			key = key[:i]
		}
		out[key] = s
	}
	return out
}
