package filter

import (
	"strings"
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
			want: "is:open is:pr repo:acme/a repo:acme/b created:>=2026-07-26T00:00:00-07:00",
		},
		{
			name: "tokyo_morning_utc_still_yesterday",
			now:  time.Date(2026, 7, 29, 8, 0, 0, 0, tokyo),
			want: "is:open is:pr repo:acme/a repo:acme/b created:>=2026-07-26T00:00:00+09:00",
		},
		{
			name: "utc_control",
			now:  time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
			want: "is:open is:pr repo:acme/a repo:acme/b created:>=2026-07-26T00:00:00Z",
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
	half := time.FixedZone("IST", 5*3600+30*60)

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
			want:  "updated:>=2026-07-28T00:00:00-07:00",
		},
		{
			name:  "los_angeles_1930_zero_days_ago",
			now:   time.Date(2026, 7, 29, 19, 30, 0, 0, la),
			query: "updated:(0 days ago)",
			want:  "updated:>=2026-07-29T00:00:00-07:00",
		},
		{
			name:  "los_angeles_1930_one_week_ago",
			now:   time.Date(2026, 7, 29, 19, 30, 0, 0, la),
			query: "closed:(1 week ago)",
			want:  "closed:>=2026-07-22T00:00:00-07:00",
		},
		{
			name:  "tokyo_0800_one_day_ago",
			now:   time.Date(2026, 7, 29, 8, 0, 0, 0, tokyo),
			query: "updated:(1 day ago)",
			want:  "updated:>=2026-07-28T00:00:00+09:00",
		},
		{
			name:  "tokyo_0800_zero_days_ago",
			now:   time.Date(2026, 7, 29, 8, 0, 0, 0, tokyo),
			query: "updated:(0 days ago)",
			want:  "updated:>=2026-07-29T00:00:00+09:00",
		},
		{
			name:  "tokyo_0800_five_hours_ago_stays_same_local_day",
			now:   time.Date(2026, 7, 29, 8, 0, 0, 0, tokyo),
			query: "updated:(5 hours ago)",
			want:  "updated:>=2026-07-29T00:00:00+09:00",
		},
		{
			name:  "half_hour_offset_zone_one_day_ago",
			now:   time.Date(2026, 7, 29, 8, 0, 0, 0, half),
			query: "updated:(1 day ago)",
			want:  "updated:>=2026-07-28T00:00:00+05:30",
		},
		{
			name:  "utc_control_one_day_ago",
			now:   time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
			query: "updated:(1 day ago)",
			want:  "updated:>=2026-07-28T00:00:00Z",
		},
		{
			name:  "utc_control_one_week_ago",
			now:   time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
			query: "closed:(1 week ago)",
			want:  "closed:>=2026-07-22T00:00:00Z",
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
			want: "repo:org/x created:>=2026-07-22T00:00:00-07:00",
		},
		{
			name: "tokyo_0800",
			now:  time.Date(2026, 7, 29, 8, 0, 0, 0, tokyo),
			want: "repo:org/x created:>=2026-07-22T00:00:00+09:00",
		},
		{
			name: "utc_control",
			now:  time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
			want: "repo:org/x created:>=2026-07-22T00:00:00Z",
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
			want: "updated:>=2026-07-28T00:00:00-07:00",
		},
		{
			name: "los_angeles_closed_three_days_ago",
			now:  time.Date(2026, 7, 29, 19, 30, 0, 0, la),
			tmpl: `{{closed "3 days ago"}}`,
			want: "closed:>=2026-07-26T00:00:00-07:00",
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
			want: "updated:>=2026-07-28T00:00:00+09:00",
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
			want: "updated:>=2026-07-28T00:00:00Z",
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

const qualifierStampContract = "a relative qualifier must emit an instant, not a bare date. GitHub search reads " +
	"a bare YYYY-MM-DD as UTC, so `updated:(1 day ago)` from Los Angeles would match from 17:00 the previous " +
	"local day: seven hours wider than asked for. This test parses the emitted value back and compares the " +
	"instant against relativeDay, so a bare date fails to parse instead of silently widening the window."

func TestRelativeQualifierEmitsTheLocalMidnightInstant(t *testing.T) {
	t.Log(qualifierStampContract)

	la := mustLoad(t, "America/Los_Angeles")
	tokyo := mustLoad(t, "Asia/Tokyo")

	for _, tc := range []struct {
		name   string
		now    time.Time
		phrase string
	}{
		{"negative_offset", time.Date(2026, 7, 29, 19, 30, 0, 0, la), "1 day ago"},
		{"positive_offset", time.Date(2026, 7, 29, 8, 0, 0, 0, tokyo), "1 day ago"},
		{"half_hour_offset", time.Date(2026, 7, 29, 8, 0, 0, 0, time.FixedZone("IST", 5*3600+30*60)), "3 days ago"},
		{"utc", time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC), "1 week ago"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want, err := relativeDay(tc.phrase, tc.now)
			if err != nil {
				t.Fatal(err)
			}
			for _, form := range []string{
				"updated:(" + tc.phrase + ")",
				`{{updated "` + tc.phrase + `"}}`,
			} {
				out, err := expandAt(form, map[string]string{}, fixedClock(tc.now))
				if err != nil {
					t.Fatalf("%s: %v", form, err)
				}
				raw, ok := strings.CutPrefix(out, "updated:>=")
				if !ok {
					t.Fatalf("%s expanded to %q, want an updated:>= qualifier", form, out)
				}
				got, err := time.Parse(time.RFC3339, raw)
				if err != nil {
					t.Fatalf("%s emitted %q, which is not an RFC3339 instant: %v\n%s", form, raw, err, qualifierStampContract)
				}
				if !got.Equal(want) {
					t.Fatalf("%s emitted %q = %s, want the local midnight instant %s\n%s",
						form, raw, got, want, qualifierStampContract)
				}
			}
		})
	}
}

func TestAgoStaysABareLocalDate(t *testing.T) {
	la := mustLoad(t, "America/Los_Angeles")
	got, err := expandAt(`after:{{ago "1 day ago"}}`, map[string]string{},
		fixedClock(time.Date(2026, 7, 29, 19, 30, 0, 0, la)))
	if err != nil {
		t.Fatal(err)
	}
	want := "after:2026-07-28"
	if got != want {
		t.Fatalf("got %q, want %q: `ago` carries no qualifier, so it feeds provider syntaxes "+
			"(slack/gmail `after:`) that accept a bare date only", got, want)
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
	want := "is:pr repo:a repo:b created:>=2026-07-27T00:00:00-07:00"
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
	want := "created:(not a relative) updated:>=2026-07-28T00:00:00-07:00"
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

const relativeDayContract = "relativeDay must return midnight in the caller's own zone, not the raw " +
	"offset arithmetic. Every current caller formats with dayLayout, so truncating to midnight cannot " +
	"change the emitted string today and startOfDay looks like dead code. It is not: it is what makes " +
	"the returned instant mean `the start of that local day`, so a caller that ever formats with a time " +
	"component cannot silently leak hour precision. This test pins the instant, not the formatted date, " +
	"so removing startOfDay fails here instead of passing everything."

func TestRelativeDayReturnsLocalMidnight(t *testing.T) {
	t.Log(relativeDayContract)

	la := mustLoad(t, "America/Los_Angeles")
	tokyo := mustLoad(t, "Asia/Tokyo")

	for _, tc := range []struct {
		name   string
		now    time.Time
		phrase string
		want   time.Time
	}{
		{"la_days", time.Date(2026, 7, 29, 19, 30, 0, 0, la), "3 days ago", time.Date(2026, 7, 26, 0, 0, 0, 0, la)},
		{"la_weeks", time.Date(2026, 7, 29, 19, 30, 0, 0, la), "1 week ago", time.Date(2026, 7, 22, 0, 0, 0, 0, la)},
		{"la_hours_same_day", time.Date(2026, 7, 29, 19, 30, 0, 0, la), "5 hours ago", time.Date(2026, 7, 29, 0, 0, 0, 0, la)},
		{"la_hours_crosses_midnight", time.Date(2026, 7, 29, 2, 30, 0, 0, la), "5 hours ago", time.Date(2026, 7, 28, 0, 0, 0, 0, la)},
		{"tokyo_days", time.Date(2026, 7, 29, 8, 0, 0, 0, tokyo), "1 day ago", time.Date(2026, 7, 28, 0, 0, 0, 0, tokyo)},
		{"tokyo_zero_days", time.Date(2026, 7, 29, 8, 0, 0, 0, tokyo), "0 days ago", time.Date(2026, 7, 29, 0, 0, 0, 0, tokyo)},
		{"utc_days", time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC), "2 days ago", time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := relativeDay(tc.phrase, tc.now)
			if err != nil {
				t.Fatal(err)
			}
			if !got.Equal(tc.want) {
				t.Fatalf("relativeDay(%q) = %s, want %s\n%s", tc.phrase, got, tc.want, relativeDayContract)
			}
			h, m, s := got.Clock()
			if h != 0 || m != 0 || s != 0 || got.Nanosecond() != 0 {
				t.Fatalf("relativeDay(%q) = %s, want midnight; startOfDay is not being applied\n%s", tc.phrase, got, relativeDayContract)
			}
			if got.Location() != tc.now.Location() {
				t.Fatalf("relativeDay(%q) location = %s, want the caller's zone %s", tc.phrase, got.Location(), tc.now.Location())
			}
		})
	}
}
