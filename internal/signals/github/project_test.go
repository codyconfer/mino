package github

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codyconfer/munin/internal/signals"
)

type fakeGraphQL struct {
	pages     []string
	viewer    string
	calls     int
	cursors   []string
	searches  []string
	teamPages []string
	teamCalls int
}

func (f *fakeGraphQL) SearchIssues(ctx context.Context, query string, perPage int) ([]byte, error) {
	return nil, nil
}

func (f *fakeGraphQL) GraphQL(ctx context.Context, query string, vars map[string]any) ([]byte, error) {
	if strings.Contains(query, "viewer") {
		return []byte(`{"data":{"viewer":{"login":"` + f.viewer + `"}}}`), nil
	}
	if strings.Contains(query, "organization") {
		if f.teamCalls >= len(f.teamPages) {
			return nil, errors.New("unexpected team call")
		}
		page := f.teamPages[f.teamCalls]
		f.teamCalls++
		return []byte(page), nil
	}
	if q, ok := vars["q"].(string); ok {
		f.searches = append(f.searches, q)
	}
	if c, ok := vars["after"].(string); ok {
		f.cursors = append(f.cursors, c)
	}
	page := f.pages[f.calls]
	f.calls++
	return []byte(page), nil
}

func searchPage(hasNext bool, cursor, nodes string) string {
	next := "false"
	if hasNext {
		next = "true"
	}
	return `{"data":{"search":{"issueCount":3,` +
		`"pageInfo":{"hasNextPage":` + next + `,"endCursor":"` + cursor + `"},` +
		`"nodes":[` + nodes + `]}}}`
}

const (
	nodeOpenedAt = "2026-07-01T10:00:00Z"
	firstReplyAt = "2026-07-18T10:00:00Z"
	lastReplyAt  = "2026-07-19T10:00:00Z"
)

func node(typename, title, status, repo string, extra string) string {
	statusJSON := "null"
	if status != "" {
		statusJSON = `{"name":"` + status + `"}`
	}
	slug := strings.ReplaceAll(title, " ", "-")
	return `{"__typename":"` + typename + `","title":"` + title + `","url":"https://github.com/` + repo + `/issues/` + slug + `",` +
		`"body":"details","createdAt":"` + nodeOpenedAt + `","updatedAt":"2026-07-20T10:00:00Z","state":"OPEN",` +
		`"repository":{"nameWithOwner":"` + repo + `"},"author":{"login":"reporter"},` +
		`"assignees":{"nodes":[{"login":"codyconfer"}]},"labels":{"nodes":[{"name":"type/bug"}]},` +
		extra +
		`"projectItems":{"nodes":[{"project":{"number":17,"owner":{"login":"acme"}},"status":` + statusJSON + `}]}}`
}

var (
	inProgressNode = node("Issue", "broken dashboard", "In Progress", "acme/escalations", "")
	prNode         = node("PullRequest", "fix query editor", "Needs Review", "acme/tooling", `"isDraft":false,`)
	incomingNode   = node("Issue", "new report", "Incoming", "acme/escalations", "")
	offBoardNode   = `{"__typename":"Issue","title":"elsewhere","url":"https://github.com/acme/other/issues/9",` +
		`"updatedAt":"2026-07-20T10:00:00Z","state":"OPEN","repository":{"nameWithOwner":"acme/other"},` +
		`"projectItems":{"nodes":[{"project":{"number":42,"owner":{"login":"acme"}},"status":{"name":"Incoming"}}]}}`

	teamReplyNode = node("Issue", "team replied", "Waiting", "acme/escalations",
		`"comments":{"nodes":[{"createdAt":"`+firstReplyAt+`","author":{"login":"custuser","__typename":"User"}},`+
			`{"createdAt":"`+lastReplyAt+`","author":{"login":"alice","__typename":"User"}}]},`)
	custReplyNode = node("Issue", "customer replied", "Waiting", "acme/escalations",
		`"comments":{"nodes":[{"createdAt":"`+firstReplyAt+`","author":{"login":"alice","__typename":"User"}},`+
			`{"createdAt":"`+lastReplyAt+`","author":{"login":"custuser","__typename":"User"}}]},`)
)

func TestParseProjectRef(t *testing.T) {
	cases := []struct {
		in     string
		owner  string
		number int
		bad    bool
	}{
		{in: "acme/17", owner: "acme", number: 17},
		{in: "https://github.com/orgs/acme/projects/17/views/5", owner: "acme", number: 17},
		{in: "https://github.com/users/codyconfer/projects/3", owner: "codyconfer", number: 3},
		{in: "orgs/acme/projects/17", owner: "acme", number: 17},
		{in: "acme", bad: true},
		{in: "acme/not-a-number", bad: true},
		{in: "", bad: true},
	}
	for _, c := range cases {
		owner, number, err := ParseProjectRef(c.in)
		if c.bad {
			if err == nil {
				t.Errorf("ParseProjectRef(%q): want error, got %s/%d", c.in, owner, number)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseProjectRef(%q): %v", c.in, err)
			continue
		}
		if owner != c.owner || number != c.number {
			t.Errorf("ParseProjectRef(%q) = %s/%d, want %s/%d", c.in, owner, number, c.owner, c.number)
		}
	}
}

func TestProjectFetchFiltersByStatus(t *testing.T) {
	be := &fakeGraphQL{pages: []string{searchPage(false, "", strings.Join([]string{inProgressNode, prNode, incomingNode}, ","))}}
	sig := NewProject(ProjectSpec{Owner: "acme", Number: 17, Filter: `status:"In Progress" repo:acme/escalations is:open -is:pr`}, be, 30, nil)

	secs, err := sig.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(secs) != 1 {
		t.Fatalf("want 1 section, got %d", len(secs))
	}
	if len(secs[0].Items) != 1 {
		t.Fatalf("want 1 item, got %d: %+v", len(secs[0].Items), secs[0].Items)
	}
	it := secs[0].Items[0]
	if it.Title != "broken dashboard" {
		t.Errorf("title = %q", it.Title)
	}
	if it.Kind != "issue" {
		t.Errorf("kind = %q, want issue", it.Kind)
	}
	if it.Meta["status"] != "In Progress" {
		t.Errorf("meta status = %q", it.Meta["status"])
	}
	if it.Subtitle != "acme/escalations · In Progress" {
		t.Errorf("subtitle = %q", it.Subtitle)
	}

	if len(be.searches) != 1 {
		t.Fatalf("want 1 search, got %v", be.searches)
	}
	got := be.searches[0]
	for _, want := range []string{"project:acme/17", "repo:acme/escalations", "is:open", "-is:pr", "sort:updated-desc"} {
		if !strings.Contains(got, want) {
			t.Errorf("search %q missing %q", got, want)
		}
	}
	if strings.Contains(got, "status:") {
		t.Errorf("status must stay local, search was %q", got)
	}
}

func TestProjectFetchSkipsItemsOnOtherBoards(t *testing.T) {
	be := &fakeGraphQL{pages: []string{searchPage(false, "", incomingNode+","+offBoardNode)}}
	sig := NewProject(ProjectSpec{Owner: "acme", Number: 17, Filter: "status:Incoming"}, be, 30, nil)

	secs, err := sig.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(secs[0].Items) != 1 || secs[0].Items[0].Title != "new report" {
		t.Fatalf("unexpected items: %+v", secs[0].Items)
	}
}

func TestProjectFetchResolvesViewer(t *testing.T) {
	be := &fakeGraphQL{
		pages:  []string{searchPage(false, "", inProgressNode+","+prNode)},
		viewer: "codyconfer",
	}
	sig := NewProject(ProjectSpec{Owner: "acme", Number: 17, Filter: `is:pr status:"Needs Review" assignee:@me`}, be, 30, nil)

	secs, err := sig.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(secs[0].Items) != 1 || secs[0].Items[0].Title != "fix query editor" {
		t.Fatalf("unexpected items: %+v", secs[0].Items)
	}
	if !strings.Contains(be.searches[0], "assignee:codyconfer") {
		t.Errorf("@me not resolved in search: %q", be.searches[0])
	}
}

func TestProjectFetchPagesAndCapsAtMax(t *testing.T) {
	be := &fakeGraphQL{pages: []string{
		searchPage(true, "cur1", incomingNode),
		searchPage(false, "", node("Issue", "second report", "Incoming", "acme/escalations", "")+","+
			node("Issue", "third report", "Incoming", "acme/escalations", "")),
	}}
	sig := NewProject(ProjectSpec{Owner: "acme", Number: 17, Filter: "status:Incoming"}, be, 2, nil)

	secs, err := sig.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(secs[0].Items) != 2 {
		t.Fatalf("want 2 items (max), got %d", len(secs[0].Items))
	}
	if be.calls != 2 {
		t.Errorf("want 2 pages fetched, got %d", be.calls)
	}
	if len(be.cursors) != 1 || be.cursors[0] != "cur1" {
		t.Errorf("cursors = %v", be.cursors)
	}
}

func TestProjectFetchScopeError(t *testing.T) {
	be := &fakeGraphQL{pages: []string{`{"errors":[{"type":"INSUFFICIENT_SCOPES","message":"needs read:project"}]}`}}
	sig := NewProject(ProjectSpec{Owner: "acme", Number: 17}, be, 30, nil)

	_, err := sig.Fetch(context.Background())
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "read:project") {
		t.Errorf("error = %v", err)
	}
}

func TestProjectFetchRejectsBadFilter(t *testing.T) {
	be := &fakeGraphQL{pages: []string{searchPage(false, "", inProgressNode)}}
	sig := NewProject(ProjectSpec{Owner: "acme", Number: 17, Filter: "column:Incoming"}, be, 30, nil)

	_, err := sig.Fetch(context.Background())
	if err == nil {
		t.Fatal("want error for unsupported qualifier")
	}
	if be.calls != 0 {
		t.Errorf("want no api calls, got %d", be.calls)
	}
}

func TestProjectSectionTitle(t *testing.T) {
	be := &fakeGraphQL{pages: []string{searchPage(false, "", inProgressNode)}}
	sig := NewProject(ProjectSpec{Owner: "acme", Number: 17, Title: "Blocked"}, be, 30, nil)
	secs, err := sig.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if secs[0].Title != "Blocked" {
		t.Errorf("section title = %q", secs[0].Title)
	}

	be = &fakeGraphQL{pages: []string{searchPage(false, "", inProgressNode)}}
	sig = NewProject(ProjectSpec{Owner: "acme", Number: 17, Filter: "status:Blocked"}, be, 30, nil)
	secs, err = sig.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if secs[0].Title != "project acme/17 · status:Blocked" {
		t.Errorf("section title = %q", secs[0].Title)
	}
}

func TestProjectFetchMarksTeamReplies(t *testing.T) {
	be := &fakeGraphQL{
		pages:     []string{searchPage(false, "", teamReplyNode+","+custReplyNode)},
		teamPages: []string{teamPage(false, "", "alice", "bob")},
	}
	sig := NewProject(ProjectSpec{Owner: "acme", Number: 17, Filter: "status:Waiting", Team: "acme/platform"}, be, 30, nil)

	secs, err := sig.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	items := secs[0].Items
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	if got := items[0].Meta["last_comment_by"]; got != "alice" {
		t.Errorf("last_comment_by = %q, want alice", got)
	}
	if got := items[0].Meta["last_comment_team"]; got != "true" {
		t.Errorf("last_comment_team = %q, want true", got)
	}
	if got := items[0].Meta["last_comment_at"]; got != lastReplyAt {
		t.Errorf("last_comment_at = %q, want %q", got, lastReplyAt)
	}
	if got := items[1].Meta["last_comment_by"]; got != "custuser" {
		t.Errorf("last_comment_by = %q, want custuser", got)
	}
	if got := items[1].Meta["last_comment_team"]; got != "false" {
		t.Errorf("last_comment_team = %q, want false", got)
	}
	if got := items[1].Meta["last_comment_at"]; got != lastReplyAt {
		t.Errorf("last_comment_at = %q, want %q", got, lastReplyAt)
	}
	if !strings.Contains(be.searches[0], "project:acme/17") {
		t.Errorf("search = %q", be.searches[0])
	}
}

func TestProjectFetchWithoutTeamOmitsTeamMeta(t *testing.T) {
	be := &fakeGraphQL{pages: []string{searchPage(false, "", custReplyNode)}}
	sig := NewProject(ProjectSpec{Owner: "acme", Number: 17, Filter: "status:Waiting"}, be, 30, nil)

	secs, err := sig.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	it := secs[0].Items[0]
	if got := it.Meta["last_comment_by"]; got != "custuser" {
		t.Errorf("last_comment_by = %q, want custuser", got)
	}
	if _, ok := it.Meta["last_comment_team"]; ok {
		t.Error("last_comment_team must be absent when no team is configured")
	}
	if got := it.Meta["last_comment_at"]; got != lastReplyAt {
		t.Errorf("last_comment_at = %q, want %q (independent of team:)", got, lastReplyAt)
	}
	if be.teamCalls != 0 {
		t.Errorf("team calls = %d, want 0", be.teamCalls)
	}
}

func TestProjectSearchQueryRequestsComments(t *testing.T) {
	if !strings.Contains(projectSearchQuery, "comments(last:5){nodes{createdAt author{login __typename}}}") {
		t.Error("project search query must request the last comments with their timestamps")
	}
	if !strings.Contains(projectSearchQuery, "createdAt updatedAt") {
		t.Error("project search query must request createdAt for the no-comment fallback")
	}
}

func TestGraphQLURL(t *testing.T) {
	cases := map[string]string{
		"":                               "https://api.github.com/graphql",
		"https://api.github.com":         "https://api.github.com/graphql",
		"https://ghe.example.com/api/v3": "https://ghe.example.com/api/graphql",
	}
	for in, want := range cases {
		if got := graphQLURL(in); got != want {
			t.Errorf("graphQLURL(%q) = %q, want %q", in, got, want)
		}
	}
}

type cursorBackend struct {
	mu       sync.Mutex
	pages    map[string]string
	cursors  []string
	calls    int
	peak     int
	live     int
	entered  chan string
	release  chan struct{}
	together int
	barrier  chan struct{}
}

func newCursorBackend(pages map[string]string) *cursorBackend {
	return &cursorBackend{pages: pages}
}

func (c *cursorBackend) SearchIssues(context.Context, string, int) ([]byte, error) { return nil, nil }

func (c *cursorBackend) GraphQL(ctx context.Context, query string, vars map[string]any) ([]byte, error) {
	after, _ := vars["after"].(string)
	c.mu.Lock()
	page, ok := c.pages[after]
	c.calls++
	c.cursors = append(c.cursors, after)
	c.live++
	c.peak = max(c.peak, c.live)
	entered, release, barrier := c.entered, c.release, c.barrier
	if barrier != nil && after != "" && c.live >= c.together {
		close(c.barrier)
		c.barrier = nil
	}
	c.mu.Unlock()
	if barrier != nil && after != "" {
		select {
		case <-barrier:
		case <-time.After(2 * time.Second):
		}
	}
	defer func() {
		c.mu.Lock()
		c.live--
		c.mu.Unlock()
	}()
	if entered != nil {
		entered <- after
		<-release
	}
	if !ok {
		return nil, errors.New("no page for cursor " + after)
	}
	return []byte(page), nil
}

func searchPageOf(count int, hasNext bool, cursor, nodes string) string {
	next := "false"
	if hasNext {
		next = "true"
	}
	return `{"data":{"search":{"issueCount":` + strconv.Itoa(count) + `,` +
		`"pageInfo":{"hasNextPage":` + next + `,"endCursor":"` + cursor + `"},` +
		`"nodes":[` + nodes + `]}}}`
}

func incoming(title string) string { return node("Issue", title, "Incoming", "acme/escalations", "") }

func TestProjectFetchBurstsPagesInParallel(t *testing.T) {
	be := newCursorBackend(map[string]string{
		"":                searchPageOf(120, true, searchCursor(50), incoming("one")),
		searchCursor(50):  searchPageOf(120, true, searchCursor(100), incoming("two")),
		searchCursor(100): searchPageOf(120, false, "", incoming("three")),
	})
	be.together, be.barrier = 2, make(chan struct{})
	sig := NewProject(ProjectSpec{Owner: "acme", Number: 17, Filter: "status:Incoming"}, be, 30, nil)

	secs, err := sig.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	var titles []string
	for _, it := range secs[0].Items {
		titles = append(titles, it.Title)
	}
	if strings.Join(titles, ",") != "one,two,three" {
		t.Fatalf("items = %v, want page order one,two,three", titles)
	}
	if be.calls != 3 {
		t.Errorf("pages fetched = %d, want 3", be.calls)
	}
	if be.peak < 2 {
		t.Errorf("peak concurrent pages = %d, want the tail pages fetched together", be.peak)
	}
}

func TestProjectFetchCrawlsPastAnUnderreportedCount(t *testing.T) {
	be := newCursorBackend(map[string]string{
		"":               searchPageOf(60, true, searchCursor(50), incoming("one")),
		searchCursor(50): searchPageOf(60, true, "deep", incoming("two")),
		"deep":           searchPageOf(60, false, "", incoming("three")),
	})
	sig := NewProject(ProjectSpec{Owner: "acme", Number: 17, Filter: "status:Incoming"}, be, 30, nil)

	secs, err := sig.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(secs[0].Items) != 3 {
		t.Fatalf("want 3 items, got %d", len(secs[0].Items))
	}
	if be.calls != 3 {
		t.Errorf("pages fetched = %d, want 3", be.calls)
	}
}

func TestProjectFetchDropsRepeatsAcrossPages(t *testing.T) {
	be := newCursorBackend(map[string]string{
		"":               searchPageOf(80, true, searchCursor(50), incoming("one")+","+incoming("two")),
		searchCursor(50): searchPageOf(80, false, "", incoming("two")+","+incoming("three")),
	})
	sig := NewProject(ProjectSpec{Owner: "acme", Number: 17, Filter: "status:Incoming"}, be, 30, nil)

	secs, err := sig.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	var titles []string
	for _, it := range secs[0].Items {
		titles = append(titles, it.Title)
	}
	if strings.Join(titles, ",") != "one,two,three" {
		t.Fatalf("items = %v, want the repeat dropped", titles)
	}
}

func TestProjectFetchSharesAWalkBetweenConcurrentQueries(t *testing.T) {
	lead := newCursorBackend(map[string]string{"": searchPageOf(10, false, "", incoming("one"))})
	lead.entered, lead.release = make(chan string), make(chan struct{})
	follow := newCursorBackend(map[string]string{"": searchPageOf(10, false, "", incoming("one"))})

	spec := ProjectSpec{Owner: "acme", Number: 17, Filter: "status:Incoming"}
	leadDone := make(chan []signals.Section, 1)
	go func() {
		secs, err := NewProject(spec, lead, 30, nil).Fetch(context.Background())
		if err != nil {
			t.Error(err)
		}
		leadDone <- secs
	}()
	<-lead.entered

	followDone := make(chan []signals.Section, 1)
	go func() {
		secs, err := NewProject(spec, follow, 30, nil).Fetch(context.Background())
		if err != nil {
			t.Error(err)
		}
		followDone <- secs
	}()

	select {
	case <-time.After(100 * time.Millisecond):
	case <-followDone:
		t.Fatal("follower finished before the leader — walk was not shared")
	}
	close(lead.release)

	leadSecs, followSecs := <-leadDone, <-followDone
	if follow.calls != 0 {
		t.Errorf("follower ran its own walk (%d pages)", follow.calls)
	}
	if len(leadSecs[0].Items) != 1 || len(followSecs[0].Items) != 1 {
		t.Fatalf("lead=%d follow=%d items, want 1 each", len(leadSecs[0].Items), len(followSecs[0].Items))
	}
}

func done(title string) string { return node("Issue", title, "Done", "acme/escalations", "") }

func TestProjectFetchStopsOnceALocalFilterIsSatisfied(t *testing.T) {
	be := newCursorBackend(map[string]string{
		"": searchPageOf(1000, true, searchCursor(50),
			strings.Join([]string{incoming("one"), incoming("two"), incoming("three")}, ",")),
	})
	sig := NewProject(ProjectSpec{Owner: "acme", Number: 17, Filter: "status:Incoming"}, be, 2, nil)

	secs, err := sig.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(secs[0].Items) != 2 {
		t.Fatalf("items = %d, want 2 (the configured max)", len(secs[0].Items))
	}
	if be.calls != 1 {
		t.Errorf("pages fetched = %d, want 1: the first page already held max items passing status:", be.calls)
	}
}

func TestProjectFetchBoundsPagesByTheObservedKeepRate(t *testing.T) {
	page := func(cursor string, keep string) string {
		return searchPageOf(1000, true, cursor, strings.Join([]string{keep, done("skip a"), done("skip b")}, ","))
	}
	be := newCursorBackend(map[string]string{
		"":                page(searchCursor(50), incoming("one")),
		searchCursor(50):  page(searchCursor(100), incoming("two")),
		searchCursor(100): page(searchCursor(150), incoming("three")),
	})
	sig := NewProject(ProjectSpec{Owner: "acme", Number: 17, Filter: "status:Incoming"}, be, 3, nil)

	secs, err := sig.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(secs[0].Items) != 3 {
		t.Fatalf("items = %d, want 3", len(secs[0].Items))
	}
	if be.calls != 3 {
		t.Errorf("pages fetched = %d, want 3 projected from one kept item per page, not the full %d-page budget",
			be.calls, searchMaxPages)
	}
}
