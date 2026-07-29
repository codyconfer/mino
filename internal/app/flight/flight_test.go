package flight

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/codyconfer/munin/internal/filter"
	"github.com/codyconfer/munin/internal/signals"
)

func TestQueryDisplayPrefersTitle(t *testing.T) {
	if got := (Query{Label: "my-open-prs", Title: "Open pull requests"}).Display(); got != "Open pull requests" {
		t.Errorf("Display = %q, want the title", got)
	}
	if got := (Query{Label: "my-open-prs"}).Display(); got != "my-open-prs" {
		t.Errorf("Display without a title = %q, want the label", got)
	}
}

type fakeSignal struct {
	name  string
	items []string
	delay time.Duration
	err   error
}

func (f fakeSignal) Name() string { return f.name }

func (f fakeSignal) Fetch(ctx context.Context) ([]signals.Section, error) {
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.err != nil {
		return nil, f.err
	}
	items := make([]signals.Item, 0, len(f.items))
	for _, title := range f.items {
		items = append(items, signals.Item{Kind: "test", Title: title, Body: title})
	}
	return []signals.Section{{Signal: f.name, Title: f.name, Items: items}}, nil
}

func mustCompile(t *testing.T, f filter.Filter) filter.Compiled {
	t.Helper()
	c, err := filter.Compile(f)
	if err != nil {
		t.Fatalf("Compile(%q) failed: %v", f.Name, err)
	}
	return c
}

func itemTitles(sections []signals.Section) []string {
	var out []string
	for _, s := range sections {
		for _, it := range s.Items {
			out = append(out, it.Title)
		}
	}
	return out
}

func TestFetchGroupsPreservesInputOrder(t *testing.T) {
	queries := []Query{
		{Label: "slow", Title: "Slow query", Src: fakeSignal{name: "slow-src", items: []string{"a"}, delay: 60 * time.Millisecond}},
		{Label: "fast", Title: "Fast query", Src: fakeSignal{name: "fast-src", items: []string{"b"}}},
		{Label: "middling", Title: "Middling query", Src: fakeSignal{name: "mid-src", items: []string{"c"}, delay: 20 * time.Millisecond}},
	}

	groups := FetchGroups(context.Background(), nil, "test", time.Second, queries, 0)
	if len(groups) != len(queries) {
		t.Fatalf("got %d groups, want %d", len(groups), len(queries))
	}
	wantQueries := []string{"slow", "fast", "middling"}
	wantTitles := []string{"Slow query", "Fast query", "Middling query"}
	wantItems := []string{"a", "b", "c"}
	for i, g := range groups {
		if g.Query != wantQueries[i] {
			t.Errorf("group %d Query = %q, want %q", i, g.Query, wantQueries[i])
		}
		if g.Title != wantTitles[i] {
			t.Errorf("group %d Title = %q, want %q", i, g.Title, wantTitles[i])
		}
		if got := itemTitles(g.Sections); len(got) != 1 || got[0] != wantItems[i] {
			t.Errorf("group %d items = %v, want [%s]", i, got, wantItems[i])
		}
	}
}

func TestFetchGroupsLabelAndTitleFallbacks(t *testing.T) {
	queries := []Query{
		{Src: fakeSignal{name: "github", items: []string{"a"}}},
		{Label: "explicit-label", Src: fakeSignal{name: "gmail", items: []string{"b"}}},
	}

	groups := FetchGroups(context.Background(), nil, "test", time.Second, queries, 0)
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}
	if groups[0].Query != "github" {
		t.Errorf("empty Label should fall back to the signal name, got %q", groups[0].Query)
	}
	if groups[0].Title != "" {
		t.Errorf("empty Title and Label should Display as empty, got %q", groups[0].Title)
	}
	if groups[1].Query != "explicit-label" {
		t.Errorf("group 1 Query = %q, want the explicit label", groups[1].Query)
	}
	if groups[1].Title != "explicit-label" {
		t.Errorf("empty Title should Display as the label, got %q", groups[1].Title)
	}
}

func TestFetchGroupsFetchErrorIsIsolated(t *testing.T) {
	boom := errors.New("boom")
	queries := []Query{
		{Label: "broken", Src: fakeSignal{name: "broken-src", err: boom}},
		{Label: "healthy", Src: fakeSignal{name: "healthy-src", items: []string{"kept"}}},
	}

	groups := FetchGroups(context.Background(), nil, "test", time.Second, queries, 0)
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}
	if len(groups[0].Sections) != 1 {
		t.Fatalf("failed query yielded %d sections, want 1", len(groups[0].Sections))
	}
	errSection := groups[0].Sections[0]
	if !errors.Is(errSection.Err, boom) {
		t.Errorf("error section Err = %v, want %v", errSection.Err, boom)
	}
	if errSection.Signal != "broken-src" || errSection.Title != "broken-src" {
		t.Errorf("error section = %+v, want signal and title of broken-src", errSection)
	}
	if got := itemTitles(groups[1].Sections); len(got) != 1 || got[0] != "kept" {
		t.Errorf("healthy query items = %v, want [kept]", got)
	}
}

func TestFetchGroupsAppliesFiltersPerSection(t *testing.T) {
	drop := mustCompile(t, filter.Filter{
		Name:  "drop-noise",
		Rules: []filter.Rule{{Field: "title", Exclude: "^noise"}},
	})
	queries := []Query{
		{Label: "filtered", Src: fakeSignal{name: "src", items: []string{"noise-one", "signal-one", "noise-two"}}, Filters: []filter.Compiled{drop}},
		{Label: "unfiltered", Src: fakeSignal{name: "src", items: []string{"noise-one", "signal-one"}}},
	}

	groups := FetchGroups(context.Background(), nil, "test", time.Second, queries, 0)
	if got := itemTitles(groups[0].Sections); !reflect.DeepEqual(got, []string{"signal-one"}) {
		t.Errorf("filtered items = %v, want [signal-one]", got)
	}
	if got := itemTitles(groups[1].Sections); !reflect.DeepEqual(got, []string{"noise-one", "signal-one"}) {
		t.Errorf("unfiltered items = %v, want both items", got)
	}
}

func equivalenceQueries() []Query {
	boom := errors.New("stable failure")
	return []Query{
		{Label: "slow", Title: "Slow", Src: fakeSignal{name: "slow-src", items: []string{"a", "b"}, delay: 30 * time.Millisecond}},
		{Src: fakeSignal{name: "no-label-src", items: []string{"c"}}},
		{Label: "broken", Src: fakeSignal{name: "broken-src", err: boom}},
		{Label: "empty", Title: "Empty", Src: fakeSignal{name: "empty-src"}},
	}
}

func TestFlattenFetchGroupsEqualsFetchQueries(t *testing.T) {
	ctx := context.Background()
	viaGroups := Flatten(FetchGroups(ctx, nil, "test", time.Second, equivalenceQueries(), 7))
	direct := FetchQueries(ctx, nil, "test", time.Second, equivalenceQueries(), 7)

	if !reflect.DeepEqual(viaGroups, direct) {
		t.Errorf("Flatten(FetchGroups(...)) = %+v, want it to equal FetchQueries(...) = %+v", viaGroups, direct)
	}
}

func TestFlattenIsNilSafe(t *testing.T) {
	if got := Flatten(nil); got != nil {
		t.Errorf("Flatten(nil) = %+v, want nil", got)
	}
	if got := Flatten([]Group{}); got != nil {
		t.Errorf("Flatten(empty) = %+v, want nil", got)
	}
	groups := []Group{
		{Query: "a"},
		{Query: "b", Sections: []signals.Section{}},
		{Query: "c", Sections: []signals.Section{{Signal: "c", Title: "c"}}},
	}
	if got := Flatten(groups); len(got) != 1 || got[0].Signal != "c" {
		t.Errorf("Flatten over sparse groups = %+v, want the single section from c", got)
	}
}
