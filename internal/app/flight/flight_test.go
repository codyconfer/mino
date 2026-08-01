package flight

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/filter"
	"github.com/codyconfer/mino/internal/signals"
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

type panickySignal struct {
	name  string
	value any
}

func (p panickySignal) Name() string { return p.name }

func (p panickySignal) Fetch(context.Context) ([]signals.Section, error) {
	panic(p.value)
}

func TestFetchGroupsRecoversPluginPanic(t *testing.T) {
	queries := []Query{
		{Label: "bad", Src: panickySignal{name: "panicker", value: "assignment to entry in nil map"}},
		{Label: "healthy", Src: fakeSignal{name: "healthy-src", items: []string{"kept"}}},
	}

	groups := FetchGroups(context.Background(), nil, "test", time.Second, queries, 0)
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}
	if len(groups[0].Sections) != 1 {
		t.Fatalf("panicking query yielded %d sections, want 1", len(groups[0].Sections))
	}
	sec := groups[0].Sections[0]
	if sec.Err == nil {
		t.Fatal("panicking query produced no Section.Err")
	}
	if sec.Signal != "panicker" || sec.Title != "panicker" {
		t.Errorf("error section = %+v, want signal and title naming panicker", sec)
	}
	msg := sec.Err.Error()
	if !strings.Contains(msg, "panicker") {
		t.Errorf("Err = %q, want the culprit signal named", msg)
	}
	if !strings.Contains(msg, "assignment to entry in nil map") {
		t.Errorf("Err = %q, want the recovered panic value included", msg)
	}
	if got := itemTitles(groups[1].Sections); len(got) != 1 || got[0] != "kept" {
		t.Errorf("healthy query items = %v, want [kept]", got)
	}
}

func TestFetchQueryRecoversPluginPanic(t *testing.T) {
	sections := FetchQuery(context.Background(), nil, "test", time.Second,
		Query{Label: "bad", Src: panickySignal{name: "panicker", value: errors.New("nil deref")}}, 0)
	if len(sections) != 1 || sections[0].Err == nil {
		t.Fatalf("FetchQuery over a panicking signal = %+v, want one error section", sections)
	}
	if !strings.Contains(sections[0].Err.Error(), "panicker") {
		t.Errorf("Err = %q, want the culprit named", sections[0].Err)
	}
}

func TestFetchQueryWithoutSourceReportsConfigError(t *testing.T) {
	sections := FetchQuery(context.Background(), nil, "test", time.Second, Query{Label: "orphan"}, 0)
	if len(sections) != 1 || sections[0].Err == nil {
		t.Fatalf("FetchQuery without a source = %+v, want one error section", sections)
	}
	if sections[0].Signal != "orphan" {
		t.Errorf("error section signal = %q, want the query label", sections[0].Signal)
	}
}

type peakCounter struct {
	mu   sync.Mutex
	cur  int
	peak int
}

func (p *peakCounter) enter() {
	p.mu.Lock()
	p.cur++
	if p.cur > p.peak {
		p.peak = p.cur
	}
	p.mu.Unlock()
}

func (p *peakCounter) exit() {
	p.mu.Lock()
	p.cur--
	p.mu.Unlock()
}

func (p *peakCounter) max() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.peak
}

type countingSignal struct {
	name    string
	counter *peakCounter
	calls   *atomic.Int64
}

func (c countingSignal) Name() string { return c.name }

func (c countingSignal) Fetch(context.Context) ([]signals.Section, error) {
	c.counter.enter()
	defer c.counter.exit()
	c.calls.Add(1)
	time.Sleep(5 * time.Millisecond)
	return []signals.Section{{Signal: c.name, Title: c.name, Items: []signals.Item{{Kind: "test", Title: c.name}}}}, nil
}

func TestFetchGroupsLimitsConcurrency(t *testing.T) {
	t.Setenv(FetchLimitEnv, "3")
	var peak peakCounter
	var calls atomic.Int64
	queries := make([]Query, 24)
	for i := range queries {
		queries[i] = Query{Label: "q" + strconv.Itoa(i), Src: countingSignal{name: "src", counter: &peak, calls: &calls}}
	}

	groups := FetchGroups(context.Background(), nil, "test", 5*time.Second, queries, 0)
	if len(groups) != len(queries) {
		t.Fatalf("got %d groups, want %d", len(groups), len(queries))
	}
	if got := calls.Load(); got != int64(len(queries)) {
		t.Errorf("ran %d fetches, want %d", got, len(queries))
	}
	if got := peak.max(); got > 3 {
		t.Errorf("peak concurrency = %d, want at most the limit of 3", got)
	}
	if got := peak.max(); got < 2 {
		t.Errorf("peak concurrency = %d, want the queries to still run in parallel", got)
	}
}

func TestFetchLimitDefaultsAndEnvOverride(t *testing.T) {
	t.Setenv(FetchLimitEnv, "")
	if got := FetchLimit(); got != DefaultFetchLimit {
		t.Errorf("FetchLimit with no env = %d, want %d", got, DefaultFetchLimit)
	}
	t.Setenv(FetchLimitEnv, "2")
	if got := FetchLimit(); got != 2 {
		t.Errorf("FetchLimit = %d, want 2", got)
	}
	for _, bad := range []string{"0", "-4", "many"} {
		t.Setenv(FetchLimitEnv, bad)
		if got := FetchLimit(); got != DefaultFetchLimit {
			t.Errorf("FetchLimit with %q = %d, want the default %d", bad, got, DefaultFetchLimit)
		}
	}
}

func TestFailureOnlyWhenEveryQueryFailed(t *testing.T) {
	boom := errors.New("exit status 4")
	item := signals.Item{Kind: "test", Title: "kept"}
	cases := []struct {
		name     string
		sections []signals.Section
		wantErr  bool
	}{
		{"no sections", nil, false},
		{"all succeeded", []signals.Section{{Signal: "a", Items: []signals.Item{item}}}, false},
		{"empty but healthy", []signals.Section{{Signal: "a"}}, false},
		{"partial failure", []signals.Section{{Signal: "a", Err: boom}, {Signal: "b", Items: []signals.Item{item}}}, false},
		{"partial failure with empty sibling", []signals.Section{{Signal: "a", Err: boom}, {Signal: "b"}}, false},
		{"every section failed", []signals.Section{{Signal: "a", Err: boom}, {Signal: "b", Err: boom}}, true},
		{"single failure", []signals.Section{{Signal: "a", Err: boom}}, true},
		{"all failed but items salvaged", []signals.Section{{Signal: "a", Err: boom, Items: []signals.Item{item}}}, false},
	}
	for _, tc := range cases {
		err := Failure(tc.sections)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%s: Failure = nil, want an error", tc.name)
				continue
			}
			if !errors.Is(err, ErrAllQueriesFailed) {
				t.Errorf("%s: Failure = %v, want it to wrap ErrAllQueriesFailed", tc.name, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: Failure = %v, want nil", tc.name, err)
		}
	}
}

func TestFailureAfterFetchGroupsWithNoAuth(t *testing.T) {
	authErr := errors.New("gh auth login: exit status 4")
	queries := []Query{
		{Label: "github", Src: fakeSignal{name: "github", err: authErr}},
		{Label: "gmail", Src: fakeSignal{name: "gmail", err: authErr}},
	}
	sections := FetchQueries(context.Background(), nil, "test", time.Second, queries, 0)
	if err := Failure(sections); err == nil {
		t.Fatal("a run where every query failed must surface an error for the exit code")
	}

	queries[1] = Query{Label: "gmail", Src: fakeSignal{name: "gmail", items: []string{"one"}}}
	sections = FetchQueries(context.Background(), nil, "test", time.Second, queries, 0)
	if err := Failure(sections); err != nil {
		t.Errorf("partial failure = %v, want nil so degraded runs still exit 0", err)
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
