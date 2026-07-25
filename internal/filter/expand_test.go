package filter

import (
	"strings"
	"testing"
	"time"
)

func TestExpandBracedAliasAndRelativeCreated(t *testing.T) {
	ctx := map[string]string{
		"REPOS_ALIAS": "repo:grafana/a repo:grafana/b",
	}
	got, err := Expand(`is:open is:pr {REPOS_ALIAS} created:(3 days ago)`, ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantDay := time.Now().AddDate(0, 0, -3).UTC().Format("2006-01-02")
	want := "is:open is:pr repo:grafana/a repo:grafana/b created:>=" + wantDay
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestExpandGoTemplate(t *testing.T) {
	ctx := map[string]string{"REPOS_1": "repo:org/x"}
	got, err := Expand(`{{.REPOS_1}} {{created "1 week ago"}}`, ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantDay := time.Now().AddDate(0, 0, -7).UTC().Format("2006-01-02")
	want := "repo:org/x created:>=" + wantDay
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestExpandUnknownBraceLeftAlone(t *testing.T) {
	got, err := Expand(`keep {NOT_AN_ALIAS} literal`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != `keep {NOT_AN_ALIAS} literal` {
		t.Fatalf("got %q", got)
	}
}

func TestExpandParamsUsesFilterAliases(t *testing.T) {
	filters := []Filter{{
		Name:    "ds-repos",
		Aliases: map[string]string{"REPOS_1": "repo:a repo:b"},
	}}
	params := map[string]string{
		"query": "is:pr {REPOS_1} created:(2 days ago)",
	}
	got, err := ExpandParams(params, filters)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got["query"], "repo:a repo:b") {
		t.Fatalf("missing repos: %q", got["query"])
	}
	if !strings.Contains(got["query"], "created:>=") {
		t.Fatalf("missing created: %q", got["query"])
	}
}

func TestTemplateContextDuplicateAliasErrors(t *testing.T) {
	_, err := TemplateContext([]Filter{
		{Name: "a", Aliases: map[string]string{"X": "one"}},
		{Name: "b", Aliases: map[string]string{"X": "two"}},
	})
	if err == nil {
		t.Fatal("expected duplicate alias error")
	}
}

func TestTemplateContextMergesKeywordsAndExternal(t *testing.T) {
	prev := ExternalKeywords
	t.Cleanup(func() { ExternalKeywords = prev })
	ExternalKeywords = func(name string) (map[string]string, bool) {
		if name != "dyn" {
			return nil, false
		}
		return map[string]string{"TODAY": "keyword"}, true
	}
	ctx, err := TemplateContext([]Filter{
		{Name: "static", Keywords: map[string]string{"TEAM": "ds"}},
		{Name: "dyn"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ctx["TEAM"] != "ds" || ctx["TODAY"] != "keyword" {
		t.Fatalf("ctx = %#v", ctx)
	}
}

func TestExpandRelativeQualifiersOnlyWhenPhraseMatches(t *testing.T) {
	got, err := Expand(`created:(not a relative) updated:(1 day ago)`, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantDay := time.Now().AddDate(0, 0, -1).UTC().Format("2006-01-02")
	want := "created:(not a relative) updated:>=" + wantDay
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestExpandMissingTemplateKeyErrors(t *testing.T) {
	_, err := Expand(`{{.MISSING}}`, map[string]string{})
	if err == nil {
		t.Fatal("expected missing key error")
	}
}
