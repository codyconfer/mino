package github

import (
	"context"
	"errors"
	"strings"
	"testing"
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
	return `{"__typename":"` + typename + `","title":"` + title + `","url":"https://github.com/` + repo + `/issues/1",` +
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
		searchPage(false, "", incomingNode+","+incomingNode),
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
