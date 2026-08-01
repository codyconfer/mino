package filter

import (
	"testing"

	"github.com/codyconfer/mino/internal/signals"
)

func items() []signals.Item {
	return []signals.Item{
		{Title: "Deploy finished", Body: "deploy ok", Meta: map[string]string{"author": "alice"}},
		{Title: "CI passed", Body: "all green", Meta: map[string]string{"author": "deploy-bot"}},
		{Title: "Incident opened", Body: "sev2 incident", Meta: map[string]string{"author": "bob"}},
	}
}

func titles(items []signals.Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Title
	}
	return out
}

func mustCompile(t *testing.T, f Filter) Compiled {
	t.Helper()
	c, err := Compile(f)
	if err != nil {
		t.Fatalf("Compile(%q): %v", f.Name, err)
	}
	return c
}

func TestInclusiveOnly(t *testing.T) {

	c := mustCompile(t, Filter{Name: "inc", Rules: []Rule{{Field: "body", Include: "incident"}}})
	got := titles(c.Apply(items()))
	want := []string{"Incident opened"}
	assertEqual(t, got, want)
}

func TestExclusiveOnly(t *testing.T) {

	c := mustCompile(t, Filter{Name: "no-bots", Rules: []Rule{{Field: "meta.author", Exclude: "(?i)bot$"}}})
	got := titles(c.Apply(items()))
	want := []string{"Deploy finished", "Incident opened"}
	assertEqual(t, got, want)
}

func TestMixedRules(t *testing.T) {

	c := mustCompile(t, Filter{Name: "mix", Rules: []Rule{
		{Field: "title", Include: "."},
		{Field: "meta.author", Exclude: "bot"},
	}})
	got := titles(c.Apply(items()))
	want := []string{"Deploy finished", "Incident opened"}
	assertEqual(t, got, want)
}

func TestExcludeWinsOverInclude(t *testing.T) {

	c := mustCompile(t, Filter{Name: "conflict", Rules: []Rule{
		{Field: "title", Include: "CI passed", Exclude: "CI passed"},
	}})
	got := c.Apply(items())
	if len(got) != 0 {
		t.Fatalf("expected exclude to win (0 items), got %v", titles(got))
	}
}

func TestDefaultFieldIsBody(t *testing.T) {

	c := mustCompile(t, Filter{Name: "def", Rules: []Rule{{Exclude: "green"}}})
	got := titles(c.Apply(items()))
	want := []string{"Deploy finished", "Incident opened"}
	assertEqual(t, got, want)
}

func TestUnknownMetaKeyDropsInclusive(t *testing.T) {

	c := mustCompile(t, Filter{Name: "m", Rules: []Rule{{Field: "meta.missing", Include: "x"}}})
	if got := c.Apply(items()); len(got) != 0 {
		t.Fatalf("expected 0 items for missing meta include, got %v", titles(got))
	}
}

func TestApplyAllChains(t *testing.T) {
	f1 := mustCompile(t, Filter{Name: "a", Rules: []Rule{{Field: "meta.author", Exclude: "bot"}}})
	f2 := mustCompile(t, Filter{Name: "b", Rules: []Rule{{Field: "title", Include: "Incident"}}})
	got := titles(ApplyAll([]Compiled{f1, f2}, items()))
	assertEqual(t, got, []string{"Incident opened"})
}

func TestBadRegexReportsName(t *testing.T) {
	_, err := Compile(Filter{Name: "broken", Rules: []Rule{{Include: "("}}})
	if err == nil {
		t.Fatal("expected error for bad regex")
	}
	if got := err.Error(); !contains(got, "broken") {
		t.Fatalf("error should name the filter, got %q", got)
	}
}

func TestCompileBindsExternalEngine(t *testing.T) {
	prev := ExternalEngine
	t.Cleanup(func() { ExternalEngine = prev })
	ExternalEngine = func(name string) (func([]signals.Item) []signals.Item, bool) {
		if name != "eng" {
			return nil, false
		}
		return func(items []signals.Item) []signals.Item {
			if len(items) == 0 {
				return items
			}
			return items[:1]
		}, true
	}
	c, err := Compile(Filter{Name: "eng"})
	if err != nil {
		t.Fatal(err)
	}
	if !c.IsEngine() {
		t.Fatal("expected engine")
	}
	got := c.Apply(items())
	if len(got) != 1 {
		t.Fatalf("got %d items", len(got))
	}
}

func assertEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
