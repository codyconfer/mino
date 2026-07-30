package filter

import (
	"testing"
	"time"
)

func mustLoad(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return loc
}

func fixedClock(at time.Time) func() time.Time {
	return func() time.Time { return at }
}

func TestExpandBracedAliasAndRelativeCreated(t *testing.T) {
	ctx := map[string]string{
		"REPOS_ALIAS": "repo:acme/a repo:acme/b",
	}
	la := mustLoad(t, "America/Los_Angeles")
	tokyo := mustLoad(t, "Asia/Tokyo")

	cases := []struct {
		name string
		now  time.Time
		want string
	}{
		{
			name: "los_angeles_evening_utc_already_tomorrow",
			now:  time.Date(2026, 7, 29, 19, 30, 0, 0, la),
			want: "is:open is:pr repo:acme/a repo:acme/b created:>=2026-07-26",
		},
		{
			name: "tokyo_morning_utc_still_yesterday",
			now:  time.Date(2026, 7, 29, 8, 0, 0, 0, tokyo),
			want: "is:open is:pr repo:acme/a repo:acme/b created:>=2026-07-26",
		},
		{
			name: "utc_control",
			now:  time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
			want: "is:open is:pr repo:acme/a repo:acme/b created:>=2026-07-26",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := expandAt(`is:open is:pr {REPOS_ALIAS} created:(3 days ago)`, ctx, fixedClock(tc.now))
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestExpandRelativeQualifierUsesLocalCalendarDay(t *testing.T) {
	la := mustLoad(t, "America/Los_Angeles")
	tokyo := mustLoad(t, "Asia/Tokyo")

	cases := []struct {
		name  string
		now   time.Time
		query string
		want  string
	}{
		{
			name:  "los_angeles_1930_one_day_ago",
			now:   time.Date(2026, 7, 29, 19, 30, 0, 0, la),
			query: "updated:(1 day ago)",
			want:  "updated:>=2026-07-28",
		},
		{
			name:  "los_angeles_1930_zero_days_ago",
			now:   time.Date(2026, 7, 29, 19, 30, 0, 0, la),
			query: "updated:(0 days ago)",
			want:  "updated:>=2026-07-29",
		},
		{
			name:  "los_angeles_1930_one_week_ago",
			now:   time.Date(2026, 7, 29, 19, 30, 0, 0, la),
			query: "closed:(1 week ago)",
			want:  "closed:>=2026-07-22",
		},
		{
			name:  "tokyo_0800_one_day_ago",
			now:   time.Date(2026, 7, 29, 8, 0, 0, 0, tokyo),
			query: "updated:(1 day ago)",
			want:  "updated:>=2026-07-28",
		},
		{
			name:  "tokyo_0800_zero_days_ago",
			now:   time.Date(2026, 7, 29, 8, 0, 0, 0, tokyo),
			query: "updated:(0 days ago)",
			want:  "updated:>=2026-07-29",
		},
		{
			name:  "tokyo_0800_five_hours_ago_stays_same_local_day",
			now:   time.Date(2026, 7, 29, 8, 0, 0, 0, tokyo),
			query: "updated:(5 hours ago)",
			want:  "updated:>=2026-07-29",
		},
		{
			name:  "utc_control_one_day_ago",
			now:   time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
			query: "updated:(1 day ago)",
			want:  "updated:>=2026-07-28",
		},
		{
			name:  "utc_control_one_week_ago",
			now:   time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
			query: "closed:(1 week ago)",
			want:  "closed:>=2026-07-22",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := expandAt(tc.query, nil, fixedClock(tc.now))
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestExpandGoTemplate(t *testing.T) {
	la := mustLoad(t, "America/Los_Angeles")
	tokyo := mustLoad(t, "Asia/Tokyo")
	ctx := map[string]string{"REPOS_1": "repo:org/x"}

	cases := []struct {
		name string
		now  time.Time
		want string
	}{
		{
			name: "los_angeles_1930",
			now:  time.Date(2026, 7, 29, 19, 30, 0, 0, la),
			want: "repo:org/x created:>=2026-07-22",
		},
		{
			name: "tokyo_0800",
			now:  time.Date(2026, 7, 29, 8, 0, 0, 0, tokyo),
			want: "repo:org/x created:>=2026-07-22",
		},
		{
			name: "utc_control",
			now:  time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
			want: "repo:org/x created:>=2026-07-22",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := expandAt(`{{.REPOS_1}} {{created "1 week ago"}}`, ctx, fixedClock(tc.now))
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestExpandTemplateFuncsUseLocalCalendarDay(t *testing.T) {
	la := mustLoad(t, "America/Los_Angeles")
	tokyo := mustLoad(t, "Asia/Tokyo")

	cases := []struct {
		name string
		now  time.Time
		tmpl string
		want string
	}{
		{
			name: "los_angeles_updated_one_day_ago",
			now:  time.Date(2026, 7, 29, 19, 30, 0, 0, la),
			tmpl: `{{updated "1 day ago"}}`,
			want: "updated:>=2026-07-28",
		},
		{
			name: "los_angeles_closed_three_days_ago",
			now:  time.Date(2026, 7, 29, 19, 30, 0, 0, la),
			tmpl: `{{closed "3 days ago"}}`,
			want: "closed:>=2026-07-26",
		},
		{
			name: "los_angeles_ago_bare_date",
			now:  time.Date(2026, 7, 29, 19, 30, 0, 0, la),
			tmpl: `{{ago "1 day ago"}}`,
			want: "2026-07-28",
		},
		{
			name: "tokyo_updated_one_day_ago",
			now:  time.Date(2026, 7, 29, 8, 0, 0, 0, tokyo),
			tmpl: `{{updated "1 day ago"}}`,
			want: "updated:>=2026-07-28",
		},
		{
			name: "tokyo_ago_bare_date",
			now:  time.Date(2026, 7, 29, 8, 0, 0, 0, tokyo),
			tmpl: `{{ago "1 day ago"}}`,
			want: "2026-07-28",
		},
		{
			name: "tokyo_ago_five_hours_stays_same_local_day",
			now:  time.Date(2026, 7, 29, 8, 0, 0, 0, tokyo),
			tmpl: `{{ago "5 hours ago"}}`,
			want: "2026-07-29",
		},
		{
			name: "utc_control_updated_one_day_ago",
			now:  time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
			tmpl: `{{updated "1 day ago"}}`,
			want: "updated:>=2026-07-28",
		},
		{
			name: "utc_control_ago_bare_date",
			now:  time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
			tmpl: `{{ago "1 day ago"}}`,
			want: "2026-07-28",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := expandAt(tc.tmpl, map[string]string{}, fixedClock(tc.now))
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got %q\nwant %q", got, tc.want)
			}
		})
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
	la := mustLoad(t, "America/Los_Angeles")
	got, err := expandParamsAt(params, filters, fixedClock(time.Date(2026, 7, 29, 19, 30, 0, 0, la)))
	if err != nil {
		t.Fatal(err)
	}
	want := "is:pr repo:a repo:b created:>=2026-07-27"
	if got["query"] != want {
		t.Fatalf("got %q\nwant %q", got["query"], want)
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
	la := mustLoad(t, "America/Los_Angeles")
	got, err := expandAt(
		`created:(not a relative) updated:(1 day ago)`,
		nil,
		fixedClock(time.Date(2026, 7, 29, 19, 30, 0, 0, la)),
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "created:(not a relative) updated:>=2026-07-28"
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
