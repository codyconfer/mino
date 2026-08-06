package slack

import (
	"reflect"
	"sort"
	"testing"
)

func TestUnfurl(t *testing.T) {
	names := map[string]string{"U1": "ada", "U2": "grace"}
	cases := []struct{ in, want string }{
		{"plain text", "plain text"},
		{"hi <@U1>", "hi @ada"},
		{"<@U1> and <@U2>", "@ada and @grace"},
		{"hi <@U9>", "hi @U9"},
		{"hi <@U9|fallback>", "hi @fallback"},
		{"see <#C1|eng>", "see #eng"},
		{"see <#C1>", "see #C1"},
		{"<!here> ping", "@here ping"},
		{"<!subteam^S1|@sre>", "@sre"},
		{"docs at <https://x.test|the docs>", "docs at the docs"},
		{"docs at <https://x.test>", "docs at https://x.test"},
		{"unclosed <@U1", "unclosed <@U1"},
		{"<>", ""},
	}
	for _, c := range cases {
		if got := unfurl(c.in, names); got != c.want {
			t.Errorf("unfurl(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestUnfurlWithoutNamesKeepsIDs(t *testing.T) {
	if got := unfurl("hi <@U1>", nil); got != "hi @U1" {
		t.Fatalf("unfurl = %q, want the raw id when no names resolved", got)
	}
}

func TestMentionIDs(t *testing.T) {
	into := map[string]bool{}
	mentionIDs("cc <@U1> and <@U2|grace>, also <#C1|eng> and <@U1>", into)

	var got []string
	for id := range into {
		got = append(got, id)
	}
	sort.Strings(got)
	want := []string{"U1", "U2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mentionIDs = %v, want %v", got, want)
	}
}

func TestFirstLineAndCap(t *testing.T) {
	if got := firstLine("  one  \ntwo"); got != "one" {
		t.Errorf("firstLine = %q", got)
	}
	if got := capRunes("abcdef", 3); got != "abc" {
		t.Errorf("capRunes = %q", got)
	}
	if got := capRunes("åéî", 2); got != "åé" {
		t.Errorf("capRunes multibyte = %q", got)
	}
}

func TestHostFromURL(t *testing.T) {
	cases := map[string]string{
		"https://myorg.slack.com/": "myorg.slack.com",
		"https://myorg.slack.com":  "myorg.slack.com",
		"http://myorg.slack.com/x": "myorg.slack.com",
		"":                         "",
	}
	for in, want := range cases {
		if got := hostFromURL(in); got != want {
			t.Errorf("hostFromURL(%q) = %q, want %q", in, got, want)
		}
	}
}
