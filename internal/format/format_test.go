package format

import (
	"errors"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/signals"
)

var testNow = time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)

func fixedNow() time.Time { return testNow }

func fixtureInput() Input {
	return Input{
		Formatter: "standup",
		Name:      "morning",
		Kind:      "flight",
		Role:      "ic",
		Now:       testNow,
		Groups: []InputGroup{
			{
				Query: "my-prs",
				Title: "My PRs",
				Sections: []signals.Section{
					{
						Signal: "github",
						Title:  "Open PRs",
						Meta:   map[string]string{"cache": "fresh"},
						Items: []signals.Item{
							{Kind: "pr", Title: "fix parser", URL: "https://x/1", Timestamp: testNow.Add(-3 * time.Hour), Meta: map[string]string{"repo": "b", "author": "ada"}},
							{Kind: "pr", Title: "add tests", URL: "https://x/2", Timestamp: testNow.Add(-30 * time.Hour), Meta: map[string]string{"repo": "a"}},
						},
					},
					{
						Signal: "gitlab",
						Title:  "Open MRs",
						Items: []signals.Item{
							{Kind: "mr", Title: "bump deps", Timestamp: testNow.Add(-10 * time.Minute), Meta: map[string]string{"repo": "a"}},
						},
					},
				},
			},
			{
				Query: "reviews",
				Title: "Reviews",
				Sections: []signals.Section{
					{
						Signal: "github",
						Title:  "Needs review",
						Items: []signals.Item{
							{Kind: "pr", Title: "refactor sink", Timestamp: testNow.Add(-2 * time.Hour)},
						},
					},
					{
						Signal: "jira",
						Title:  "Tickets",
						Err:    errors.New("jira unreachable"),
					},
				},
			},
		},
	}
}

func TestBuildViewsAreConsistent(t *testing.T) {
	r := Build(fixtureInput())

	if len(r.Queries) != 2 {
		t.Fatalf("Queries = %d, want 2", len(r.Queries))
	}
	if len(r.Sections) != 4 {
		t.Fatalf("Sections = %d, want 4", len(r.Sections))
	}
	if len(r.Items) != 4 {
		t.Fatalf("Items = %d, want 4", len(r.Items))
	}

	var flat []Item
	for _, g := range r.Queries {
		var groupFlat []Item
		for _, s := range g.Sections {
			groupFlat = append(groupFlat, s.Items...)
		}
		if len(groupFlat) != len(g.Items) {
			t.Errorf("group %q: Items = %d, want %d", g.Query, len(g.Items), len(groupFlat))
		}
		flat = append(flat, groupFlat...)
	}
	if len(flat) != len(r.Items) {
		t.Errorf("flattened groups = %d, Report.Items = %d", len(flat), len(r.Items))
	}
	for i := range flat {
		if flat[i].Title != r.Items[i].Title {
			t.Errorf("item %d: group view %q, flat view %q", i, flat[i].Title, r.Items[i].Title)
		}
	}
}

func TestBuildCounts(t *testing.T) {
	r := Build(fixtureInput())

	if r.Count != 4 {
		t.Errorf("Report.Count = %d, want 4", r.Count)
	}
	wantGroups := map[string]int{"my-prs": 3, "reviews": 1}
	for _, g := range r.Queries {
		if g.Count != wantGroups[g.Query] {
			t.Errorf("group %q Count = %d, want %d", g.Query, g.Count, wantGroups[g.Query])
		}
		total := 0
		for _, s := range g.Sections {
			if s.Count != len(s.Items) {
				t.Errorf("section %q Count = %d, want %d", s.Signal, s.Count, len(s.Items))
			}
			total += s.Count
		}
		if total != g.Count {
			t.Errorf("group %q Count = %d, sections total = %d", g.Query, g.Count, total)
		}
	}
}

func TestBuildStampsQueryAndSignal(t *testing.T) {
	r := Build(fixtureInput())

	for _, s := range r.Sections {
		if s.Query == "" {
			t.Errorf("section %q has empty Query", s.Title)
		}
		if s.Signal == "" {
			t.Errorf("section %q has empty Signal", s.Title)
		}
		for _, it := range s.Items {
			if it.Query != s.Query {
				t.Errorf("item %q Query = %q, want %q", it.Title, it.Query, s.Query)
			}
			if it.Signal != s.Signal {
				t.Errorf("item %q Signal = %q, want %q", it.Title, it.Signal, s.Signal)
			}
		}
	}
	for _, it := range r.Items {
		if it.Query == "" || it.Signal == "" {
			t.Errorf("flat item %q missing Query/Signal (%q/%q)", it.Title, it.Query, it.Signal)
		}
	}
}

func TestBuildPassesThroughFields(t *testing.T) {
	r := Build(fixtureInput())
	if r.Formatter != "standup" || r.Name != "morning" || r.Kind != "flight" || r.Role != "ic" {
		t.Errorf("header fields = %q/%q/%q/%q", r.Formatter, r.Name, r.Kind, r.Role)
	}
	if !r.Now.Equal(testNow) {
		t.Errorf("Now = %v, want %v", r.Now, testNow)
	}
	sec := r.Sections[0]
	if sec.Meta["cache"] != "fresh" {
		t.Errorf("section Meta not carried over: %v", sec.Meta)
	}
	it := sec.Items[0]
	if it.URL != "https://x/1" || it.Kind != "pr" || it.Meta["author"] != "ada" {
		t.Errorf("item not carried over: %+v", it)
	}
}

func TestBuildCollectsErrors(t *testing.T) {
	r := Build(fixtureInput())
	if len(r.Errors) != 1 {
		t.Fatalf("Errors = %v, want 1 entry", r.Errors)
	}
	if r.Errors[0] != "jira unreachable" {
		t.Errorf("Errors[0] = %q", r.Errors[0])
	}
	var errSec Section
	for _, s := range r.Sections {
		if s.Signal == "jira" {
			errSec = s
		}
	}
	if errSec.Err != "jira unreachable" {
		t.Errorf("section Err = %q", errSec.Err)
	}
	if errSec.Count != 0 {
		t.Errorf("errored section Count = %d, want 0", errSec.Count)
	}
}

func TestBuildZeroInput(t *testing.T) {
	r := Build(Input{})
	if len(r.Queries) != 0 || len(r.Sections) != 0 || len(r.Items) != 0 || len(r.Errors) != 0 {
		t.Errorf("zero Input produced data: %+v", r)
	}
	if r.Count != 0 {
		t.Errorf("Count = %d, want 0", r.Count)
	}
	if r.Now.IsZero() {
		t.Error("Now was not defaulted")
	}
	if _, err := Render("t", "{{ .Count }}/{{ len .Items }}", r); err != nil {
		t.Fatalf("render of zero report: %v", err)
	}
}

func TestBuildNilSectionsAndItems(t *testing.T) {
	in := Input{Groups: []InputGroup{
		{Query: "q"},
		{Query: "q2", Sections: []signals.Section{{Signal: "s"}}},
	}}
	r := Build(in)
	if len(r.Queries) != 2 {
		t.Fatalf("Queries = %d", len(r.Queries))
	}
	if r.Queries[0].Count != 0 || r.Queries[1].Count != 0 || r.Count != 0 {
		t.Errorf("counts should be zero: %+v", r)
	}
	if len(r.Sections) != 1 {
		t.Errorf("Sections = %d, want 1", len(r.Sections))
	}
}

func TestBuildDefaultsNow(t *testing.T) {
	before := time.Now()
	r := Build(Input{})
	if r.Now.Before(before) {
		t.Errorf("Now = %v, want >= %v", r.Now, before)
	}
}

func render(t *testing.T, src string, data Report) string {
	t.Helper()
	out, err := Render("t", src, data)
	if err != nil {
		t.Fatalf("Render(%q) err = %v", src, err)
	}
	return out
}

func TestFuncNowAndDate(t *testing.T) {
	fm := FuncMap(fixedNow)
	got := fm["now"].(func() time.Time)()
	if !got.Equal(testNow) {
		t.Errorf("now = %v, want %v", got, testNow)
	}
	d := fm["date"].(func(string, time.Time) string)
	if s := d("2006-01-02", testNow); s != "2026-07-29" {
		t.Errorf("date = %q", s)
	}
}

func TestFuncNowPipesLast(t *testing.T) {
	tmpl, err := parseWith(fixedNow, `{{ now | date "2006-01-02" }}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, Report{}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if b.String() != "2026-07-29" {
		t.Errorf("got %q, want 2026-07-29", b.String())
	}
}

func TestFuncRel(t *testing.T) {
	rel := FuncMap(fixedNow)["rel"].(func(time.Time) string)
	cases := []struct {
		in   time.Time
		want string
	}{
		{testNow.Add(-3 * time.Hour), "3h ago"},
		{testNow.Add(-45 * time.Minute), "45m ago"},
		{testNow.Add(-50 * time.Hour), "2d ago"},
		{testNow, "just now"},
		{testNow.Add(2 * time.Hour), "in 2h"},
		{time.Time{}, ""},
	}
	for _, c := range cases {
		if got := rel(c.in); got != c.want {
			t.Errorf("rel(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFuncMeta(t *testing.T) {
	meta := FuncMap(fixedNow)["meta"].(func(string, map[string]string) string)
	m := map[string]string{"repo": "munin"}
	if got := meta("repo", m); got != "munin" {
		t.Errorf("meta hit = %q", got)
	}
	if got := meta("nope", m); got != "" {
		t.Errorf("meta miss = %q, want empty", got)
	}
	if got := meta("repo", nil); got != "" {
		t.Errorf("meta on nil map = %q, want empty", got)
	}
}

func TestFuncDefault(t *testing.T) {
	def := FuncMap(fixedNow)["default"].(func(string, string) string)
	if got := def("none", ""); got != "none" {
		t.Errorf("default empty = %q", got)
	}
	if got := def("none", "set"); got != "set" {
		t.Errorf("default set = %q", got)
	}
}

func TestFuncStringHelpers(t *testing.T) {
	fm := FuncMap(fixedNow)
	if got := fm["trim"].(func(string) string)("  hi\n"); got != "hi" {
		t.Errorf("trim = %q", got)
	}
	if got := fm["upper"].(func(string) string)("aB"); got != "AB" {
		t.Errorf("upper = %q", got)
	}
	if got := fm["lower"].(func(string) string)("aB"); got != "ab" {
		t.Errorf("lower = %q", got)
	}
	title := fm["title"].(func(string) string)
	for in, want := range map[string]string{
		"hello world": "Hello World",
		"":            "",
		"open prs":    "Open Prs",
		"a-b c":       "A-B C",
	} {
		if got := title(in); got != want {
			t.Errorf("title(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFuncJoin(t *testing.T) {
	join := FuncMap(fixedNow)["join"].(func(string, []string) string)
	if got := join(", ", []string{"a", "b"}); got != "a, b" {
		t.Errorf("join = %q", got)
	}
	if got := join(", ", nil); got != "" {
		t.Errorf("join nil = %q", got)
	}
}

func TestFuncIndent(t *testing.T) {
	ind := FuncMap(fixedNow)["indent"].(func(int, string) string)
	if got := ind(2, "a\nb"); got != "  a\n  b" {
		t.Errorf("indent = %q", got)
	}
	if got := ind(0, "a"); got != "a" {
		t.Errorf("indent 0 = %q", got)
	}
	if got := ind(-1, "a"); got != "a" {
		t.Errorf("indent -1 = %q", got)
	}
	if got := ind(1, ""); got != " " {
		t.Errorf("indent empty = %q", got)
	}
}

func TestFuncTruncate(t *testing.T) {
	tr := FuncMap(fixedNow)["truncate"].(func(int, string) string)
	cases := []struct {
		n    int
		in   string
		want string
	}{
		{10, "short", "short"},
		{5, "short", "short"},
		{4, "short", "shor…"},
		{3, "héllo", "hél…"},
		{0, "abc", "…"},
		{0, "", ""},
		{-1, "abc", "abc"},
		{2, "日本語", "日本…"},
	}
	for _, c := range cases {
		if got := tr(c.n, c.in); got != c.want {
			t.Errorf("truncate(%d, %q) = %q, want %q", c.n, c.in, got, c.want)
		}
	}
}

func TestFuncCount(t *testing.T) {
	cnt := FuncMap(fixedNow)["count"].(func(any) int)
	cases := []struct {
		in   any
		want int
	}{
		{nil, 0},
		{[]Item(nil), 0},
		{[]Item{{}, {}}, 2},
		{map[string]string{"a": "1"}, 1},
		{"abcd", 4},
		{42, 0},
	}
	for _, c := range cases {
		if got := cnt(c.in); got != c.want {
			t.Errorf("count(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestFuncLimit(t *testing.T) {
	lim := FuncMap(fixedNow)["limit"].(func(int, []Item) []Item)
	items := []Item{{Title: "a"}, {Title: "b"}, {Title: "c"}}
	if got := lim(2, items); len(got) != 2 || got[1].Title != "b" {
		t.Errorf("limit 2 = %+v", got)
	}
	if got := lim(9, items); len(got) != 3 {
		t.Errorf("limit beyond len = %d, want 3", len(got))
	}
	if got := lim(0, items); len(got) != 0 {
		t.Errorf("limit 0 = %d, want 0", len(got))
	}
	if got := lim(-1, items); len(got) != 3 {
		t.Errorf("limit -1 = %d, want 3", len(got))
	}
	if got := lim(2, nil); len(got) != 0 {
		t.Errorf("limit on nil = %d", len(got))
	}
}

func TestFuncByMeta(t *testing.T) {
	by := FuncMap(fixedNow)["byMeta"].(func(string, []Item) []Bucket)
	items := []Item{
		{Title: "1", Meta: map[string]string{"repo": "b"}},
		{Title: "2", Meta: map[string]string{"repo": "a"}},
		{Title: "3"},
		{Title: "4", Meta: map[string]string{"repo": "a"}},
	}
	got := by("repo", items)
	if len(got) != 3 {
		t.Fatalf("buckets = %d, want 3: %+v", len(got), got)
	}
	if got[0].Key != "" || len(got[0].Items) != 1 || got[0].Items[0].Title != "3" {
		t.Errorf("missing-key bucket = %+v", got[0])
	}
	if got[1].Key != "a" || len(got[1].Items) != 2 {
		t.Errorf("bucket a = %+v", got[1])
	}
	if got[1].Items[0].Title != "2" || got[1].Items[1].Title != "4" {
		t.Errorf("bucket a lost input order: %+v", got[1].Items)
	}
	if got[2].Key != "b" || len(got[2].Items) != 1 {
		t.Errorf("bucket b = %+v", got[2])
	}
	if got := by("repo", nil); len(got) != 0 {
		t.Errorf("byMeta nil = %+v", got)
	}
}

func TestFuncWithMeta(t *testing.T) {
	with := FuncMap(fixedNow)["withMeta"].(func(string, string, []Item) []Item)
	items := []Item{
		{Title: "1", Meta: map[string]string{"kind": "pr"}},
		{Title: "2", Meta: map[string]string{"kind": "issue"}},
		{Title: "3"},
	}
	got := with("kind", "pr", items)
	if len(got) != 1 || got[0].Title != "1" {
		t.Errorf("withMeta = %+v", got)
	}
	if got := with("kind", "", items); len(got) != 1 || got[0].Title != "3" {
		t.Errorf("withMeta empty value = %+v", got)
	}
	if got := with("kind", "pr", nil); len(got) != 0 {
		t.Errorf("withMeta nil = %+v", got)
	}
}

func TestFuncSortByTimeDoesNotMutate(t *testing.T) {
	sortFn := FuncMap(fixedNow)["sortByTime"].(func([]Item) []Item)
	items := []Item{
		{Title: "old", Timestamp: testNow.Add(-10 * time.Hour)},
		{Title: "new", Timestamp: testNow},
		{Title: "mid", Timestamp: testNow.Add(-1 * time.Hour)},
	}
	got := sortFn(items)
	want := []string{"new", "mid", "old"}
	for i, w := range want {
		if got[i].Title != w {
			t.Errorf("sorted[%d] = %q, want %q", i, got[i].Title, w)
		}
	}
	if items[0].Title != "old" || items[1].Title != "new" || items[2].Title != "mid" {
		t.Errorf("input was mutated: %+v", items)
	}
	if got := sortFn(nil); len(got) != 0 {
		t.Errorf("sortByTime nil = %+v", got)
	}
}

func TestMissingMetaKeyRendersEmpty(t *testing.T) {
	r := Build(fixtureInput())
	out := render(t, `[{{ (index .Items 1).Meta.author }}]`, r)
	if out != "[]" {
		t.Errorf("missing meta key rendered %q, want []", out)
	}
	out = render(t, `[{{ range .Items }}{{ .Meta.nope }}{{ end }}]`, r)
	if out != "[]" {
		t.Errorf("missing meta key over range rendered %q", out)
	}
}

func TestMissingStructFieldStillErrors(t *testing.T) {
	_, err := Render("t", `{{ .Nope }}`, Build(fixtureInput()))
	if err == nil {
		t.Fatal("want an error for a struct-field typo")
	}
	if errs.KindOf(err) != errs.KindConfig {
		t.Errorf("kind = %q, want config", errs.KindOf(err))
	}
	if !strings.Contains(err.Error(), "executing formatter") {
		t.Errorf("err = %v, want it to mention executing", err)
	}
}

func TestParseMalformed(t *testing.T) {
	_, err := Parse("bad", `{{ if .Count }}unclosed`)
	if err == nil {
		t.Fatal("want a parse error")
	}
	if errs.KindOf(err) != errs.KindConfig {
		t.Errorf("kind = %q, want config", errs.KindOf(err))
	}
	if !strings.Contains(err.Error(), `parsing formatter "bad"`) {
		t.Errorf("err = %v", err)
	}
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Error("err is not an *errs.Error")
	}
}

func TestParseUnknownFuncErrors(t *testing.T) {
	if _, err := Parse("bad", `{{ nosuchfunc 1 }}`); err == nil {
		t.Fatal("want a parse error for an unknown func")
	}
}

func TestParseOK(t *testing.T) {
	tmpl, err := Parse("ok", `{{ .Name }}`)
	if err != nil {
		t.Fatalf("Parse err = %v", err)
	}
	if tmpl == nil {
		t.Fatal("Parse returned nil template")
	}
	if tmpl.Name() != "ok" {
		t.Errorf("template name = %q, want ok", tmpl.Name())
	}
}

const standupTemplate = `# {{ .Name }} — {{ date "2006-01-02" .Now }}

{{ range .Queries }}## {{ .Title }} ({{ .Count }})
{{ range .Sections }}### {{ .Title }}
{{ if .Err }}- error: {{ .Err }}
{{ else if not .Items }}- nothing
{{ else }}{{ range sortByTime .Items }}- [{{ truncate 20 .Title }}]({{ default "-" .URL }}) {{ rel .Timestamp }}{{ with .Meta.repo }} ({{ . }}){{ end }}
{{ end }}{{ end }}
{{ end }}{{ end }}total: {{ .Count }}{{ with .Errors }}
errors: {{ join "; " . }}{{ end }}
`

func TestRenderStandup(t *testing.T) {
	r := Build(fixtureInput())
	tmpl, err := parseWith(fixedNow, standupTemplate)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, r); err != nil {
		t.Fatalf("execute: %v", err)
	}
	want := `# morning — 2026-07-29

## My PRs (3)
### Open PRs
- [fix parser](https://x/1) 3h ago (b)
- [add tests](https://x/2) 1d ago (a)

### Open MRs
- [bump deps](-) 10m ago (a)

## Reviews (1)
### Needs review
- [refactor sink](-) 2h ago

### Tickets
- error: jira unreachable

total: 4
errors: jira unreachable
`
	if b.String() != want {
		t.Errorf("render mismatch\n--- got ---\n%s\n--- want ---\n%s", b.String(), want)
	}
}

func TestRenderUsesLiveNowByDefault(t *testing.T) {
	out, err := Render("t", `{{ if now.IsZero }}zero{{ else }}set{{ end }}`, Report{})
	if err != nil {
		t.Fatalf("Render err = %v", err)
	}
	if out != "set" {
		t.Errorf("got %q, want set", out)
	}
}

func TestFuncMapNilNow(t *testing.T) {
	fm := FuncMap(nil)
	if fm["now"].(func() time.Time)().IsZero() {
		t.Error("FuncMap(nil) now returned the zero time")
	}
}

func parseWith(now func() time.Time, src string) (*template.Template, error) {
	return template.New("t").Option("missingkey=zero").Funcs(FuncMap(now)).Parse(src)
}
