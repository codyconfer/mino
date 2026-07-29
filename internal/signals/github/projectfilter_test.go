package github

import (
	"testing"

	"github.com/codyconfer/munin/internal/signals"
)

func testItem(mutate func(*projectItem)) projectItem {
	it := projectItem{
		status:    "In Progress",
		repo:      "acme/escalations",
		author:    "reporter",
		assignees: []string{"codyconfer"},
		labels:    []string{"type/bug"},
		state:     "open",
		kind:      "issue",
	}
	it.item = signals.Item{Title: "broken dashboard", Body: "details here"}
	if mutate != nil {
		mutate(&it)
	}
	return it
}

func TestProjectFilterKeeps(t *testing.T) {
	cases := []struct {
		name   string
		filter string
		item   projectItem
		viewer string
		want   bool
	}{
		{name: "status match", filter: `status:"In Progress"`, item: testItem(nil), want: true},
		{name: "status miss", filter: "status:Blocked", item: testItem(nil), want: false},
		{name: "status list", filter: `status:Blocked,"In Progress"`, item: testItem(nil), want: true},
		{name: "negated status", filter: `-status:Prioritized,"Needs Review",Docs`, item: testItem(nil), want: true},
		{name: "negated status hit", filter: `-status:"In Progress"`, item: testItem(nil), want: false},
		{name: "board view filter", filter: `-status:Prioritized,"Needs Review",Docs,"Product Review",Dependencies,"[Legacy] Prioritized Bugs" repo:acme/escalations is:open -is:pr`, item: testItem(nil), want: true},
		{name: "board view filter excludes pr", filter: `repo:acme/escalations is:open -is:pr`, item: testItem(func(it *projectItem) { it.kind = "pr" }), want: false},
		{name: "repo miss", filter: "repo:acme/other", item: testItem(nil), want: false},
		{name: "repo bare name", filter: "repo:escalations", item: testItem(nil), want: true},
		{name: "closed", filter: "is:closed", item: testItem(func(it *projectItem) { it.state = "closed" }), want: true},
		{name: "merged counts as closed", filter: "is:closed", item: testItem(func(it *projectItem) { it.state = "merged" }), want: true},
		{name: "draft", filter: "is:draft", item: testItem(func(it *projectItem) { it.draft = true }), want: true},
		{name: "assignee viewer", filter: "assignee:@me", item: testItem(nil), viewer: "codyconfer", want: true},
		{name: "assignee viewer miss", filter: "assignee:@me", item: testItem(nil), viewer: "someone-else", want: false},
		{name: "author literal", filter: "author:reporter", item: testItem(nil), want: true},
		{name: "label", filter: "label:type/bug,", item: testItem(nil), want: true},
		{name: "no assignee", filter: "no:assignee", item: testItem(func(it *projectItem) { it.assignees = nil }), want: true},
		{name: "no assignee miss", filter: "no:assignee", item: testItem(nil), want: false},
		{name: "no status", filter: "no:status", item: testItem(func(it *projectItem) { it.status = "" }), want: true},
		{name: "free text", filter: "dashboard", item: testItem(nil), want: true},
		{name: "free text miss", filter: "nonsense", item: testItem(nil), want: false},
		{name: "empty filter keeps all", filter: "", item: testItem(nil), want: true},
		{name: "case insensitive status", filter: "status:in progress", item: testItem(nil), want: false},
		{name: "quoted case insensitive", filter: `status:"in PROGRESS"`, item: testItem(nil), want: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pf, err := parseProjectFilter(c.filter)
			if err != nil {
				t.Fatalf("parse %q: %v", c.filter, err)
			}
			if got := pf.keeps(c.item, c.viewer); got != c.want {
				t.Errorf("keeps() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestProjectFilterNeedsViewer(t *testing.T) {
	for _, f := range []string{"assignee:@me", "author:@me", `status:Docs assignee:@me`} {
		pf, err := parseProjectFilter(f)
		if err != nil {
			t.Fatalf("parse %q: %v", f, err)
		}
		if !pf.needsViewer {
			t.Errorf("%q: needsViewer = false", f)
		}
	}
	pf, err := parseProjectFilter("assignee:codyconfer")
	if err != nil {
		t.Fatal(err)
	}
	if pf.needsViewer {
		t.Error("literal assignee should not need viewer")
	}
}

func TestProjectFilterErrors(t *testing.T) {
	for _, f := range []string{"column:Incoming", "status:", "is:reviewed", "no:milestone", "-project:foo"} {
		if _, err := parseProjectFilter(f); err == nil {
			t.Errorf("parseProjectFilter(%q): want error", f)
		}
	}
}

func TestSplitProjectValues(t *testing.T) {
	got := splitProjectValues(`Dependencies,"Needs Review","In Progress",Waiting,Docs`)
	want := []string{"Dependencies", "Needs Review", "In Progress", "Waiting", "Docs"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("value %d = %q, want %q", i, got[i], want[i])
		}
	}
}
