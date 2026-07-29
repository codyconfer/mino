package github

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/munin/internal/errs"
)

type fakeCache struct {
	entries map[string]string
	puts    int
	gets    int
}

func newFakeCache() *fakeCache { return &fakeCache{entries: map[string]string{}} }

func (c *fakeCache) Get(ctx context.Context, namespace, key string) (string, bool) {
	c.gets++
	v, ok := c.entries[namespace+"|"+key]
	return v, ok
}

func (c *fakeCache) Put(ctx context.Context, namespace, key, value string, expiry time.Time) {
	c.puts++
	c.entries[namespace+"|"+key] = value
}

func teamPage(hasNext bool, cursor string, logins ...string) string {
	next := "false"
	if hasNext {
		next = "true"
	}
	nodes := make([]string, 0, len(logins))
	for _, l := range logins {
		nodes = append(nodes, `{"login":"`+l+`"}`)
	}
	return `{"data":{"organization":{"team":{"members":{` +
		`"pageInfo":{"hasNextPage":` + next + `,"endCursor":"` + cursor + `"},` +
		`"nodes":[` + strings.Join(nodes, ",") + `]}}}}}`
}

func TestParseTeamRef(t *testing.T) {
	cases := []struct {
		in   string
		org  string
		slug string
		bad  bool
	}{
		{in: "acme/platform", org: "acme", slug: "platform"},
		{in: " acme/platform ", org: "acme", slug: "platform"},
		{in: "https://github.com/orgs/acme/teams/platform", org: "acme", slug: "platform"},
		{in: "acme", bad: true},
		{in: "acme/", bad: true},
		{in: "", bad: true},
	}
	for _, c := range cases {
		org, slug, err := ParseTeamRef(c.in)
		if c.bad {
			if err == nil {
				t.Errorf("ParseTeamRef(%q): want error, got %s/%s", c.in, org, slug)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseTeamRef(%q): %v", c.in, err)
			continue
		}
		if org != c.org || slug != c.slug {
			t.Errorf("ParseTeamRef(%q) = %s/%s, want %s/%s", c.in, org, slug, c.org, c.slug)
		}
	}
}

func TestResolveTeamSinglePage(t *testing.T) {
	be := &fakeGraphQL{teamPages: []string{teamPage(false, "", "Alice", "bob")}}
	roster, err := ResolveTeam(context.Background(), be, nil, "acme/platform")
	if err != nil {
		t.Fatalf("ResolveTeam: %v", err)
	}
	if !roster.Configured() {
		t.Error("roster should be configured")
	}
	if !roster.Has("alice") || !roster.Has("BOB") {
		t.Error("membership lookup should be case-insensitive")
	}
	if roster.Has("custuser") {
		t.Error("non-member reported as member")
	}
	if be.teamCalls != 1 {
		t.Errorf("team calls = %d, want 1", be.teamCalls)
	}
}

func TestResolveTeamPages(t *testing.T) {
	be := &fakeGraphQL{teamPages: []string{
		teamPage(true, "cur1", "alice"),
		teamPage(false, "", "bob"),
	}}
	roster, err := ResolveTeam(context.Background(), be, nil, "acme/platform")
	if err != nil {
		t.Fatalf("ResolveTeam: %v", err)
	}
	if !roster.Has("alice") || !roster.Has("bob") {
		t.Error("both pages should be merged into the roster")
	}
	if be.teamCalls != 2 {
		t.Errorf("team calls = %d, want 2", be.teamCalls)
	}
}

func TestResolveTeamCacheHitSkipsAPI(t *testing.T) {
	cache := newFakeCache()
	be := &fakeGraphQL{teamPages: []string{teamPage(false, "", "alice")}}
	if _, err := ResolveTeam(context.Background(), be, cache, "acme/platform"); err != nil {
		t.Fatalf("ResolveTeam: %v", err)
	}
	if cache.puts != 1 {
		t.Fatalf("cache puts = %d, want 1", cache.puts)
	}

	roster, err := ResolveTeam(context.Background(), be, cache, "acme/platform")
	if err != nil {
		t.Fatalf("ResolveTeam (cached): %v", err)
	}
	if !roster.Has("alice") {
		t.Error("cached roster lost its members")
	}
	if be.teamCalls != 1 {
		t.Errorf("team calls = %d, want 1 (second resolve should hit the cache)", be.teamCalls)
	}
}

func TestResolveTeamMissingTeam(t *testing.T) {
	be := &fakeGraphQL{teamPages: []string{`{"data":{"organization":{"team":null}}}`}}
	_, err := ResolveTeam(context.Background(), be, nil, "acme/nope")
	if err == nil {
		t.Fatal("want error for missing team")
	}
	if !strings.Contains(err.Error(), "acme/nope") {
		t.Errorf("error = %v", err)
	}
}

func TestResolveTeamScopeError(t *testing.T) {
	be := &fakeGraphQL{teamPages: []string{`{"errors":[{"type":"INSUFFICIENT_SCOPES","message":"missing scope"}]}`}}
	_, err := ResolveTeam(context.Background(), be, nil, "acme/platform")
	if err == nil {
		t.Fatal("want error")
	}
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("error = %v, want *errs.Error", err)
	}
	if e.Kind != errs.KindAuth {
		t.Errorf("kind = %q, want %q", e.Kind, errs.KindAuth)
	}
	if !strings.Contains(e.Hint, "read:org") {
		t.Errorf("hint = %q, want a read:org hint", e.Hint)
	}
}

func TestResolveTeamBadRefMakesNoCalls(t *testing.T) {
	be := &fakeGraphQL{}
	if _, err := ResolveTeam(context.Background(), be, nil, "platform"); err == nil {
		t.Fatal("want error for bad team ref")
	}
	if be.teamCalls != 0 {
		t.Errorf("team calls = %d, want 0", be.teamCalls)
	}
}

func TestNilRosterIsSafe(t *testing.T) {
	var r *Roster
	if r.Configured() {
		t.Error("nil roster should not be configured")
	}
	if r.Has("alice") {
		t.Error("nil roster should have no members")
	}
}
