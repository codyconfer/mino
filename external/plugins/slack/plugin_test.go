package slack

import (
	"context"
	"strings"
	"testing"

	"github.com/codyconfer/mino/plugin"
)

func TestBuildQueryWithNoParamsSucceeds(t *testing.T) {
	t.Setenv("SLACK_TOKEN", "xoxp-test")

	q, err := BuildQuery(newTestHost(nil, nil))
	if err != nil {
		t.Fatalf("BuildQuery with no params = %v; internal/signals/build/detail.go builds the query with nil params, so this must succeed or `mino slack show` can never reach Detail", err)
	}
	if q == nil {
		t.Fatal("BuildQuery returned no query")
	}
	if _, ok := q.(plugin.Detailer); !ok {
		t.Fatal("the query does not implement plugin.Detailer, but the descriptor advertises CapDetail")
	}
}

func TestFetchWithNoSurfaceErrorsWithHint(t *testing.T) {
	t.Setenv("SLACK_TOKEN", "xoxp-test")

	q, err := BuildQuery(newTestHost(nil, nil))
	if err != nil {
		t.Fatalf("BuildQuery = %v", err)
	}
	_, err = q.Fetch(context.Background())
	if err == nil {
		t.Fatal("Fetch with no channel/mentions/dms/search succeeded; the missing-surface error must surface at fetch time")
	}
	if !strings.Contains(err.Error(), "nothing to read") {
		t.Fatalf("error does not name the problem: %v", err)
	}
	hinted, ok := err.(interface{ Hint() string })
	if !ok || hinted.Hint() == "" {
		t.Fatalf("error carries no hint telling the user which flag to pass: %v", err)
	}
}

func TestBuildQuerySurfacesFromParams(t *testing.T) {
	t.Setenv("SLACK_TOKEN", "xoxp-test")

	q, err := BuildQuery(newTestHost(map[string]string{
		"channel":  "eng, #alerts",
		"mentions": "true",
		"dms":      "3",
		"search":   "deploy",
	}, nil))
	if err != nil {
		t.Fatalf("BuildQuery = %v", err)
	}
	s := q.(*slackSignal)
	if got := len(s.channels); got != 2 {
		t.Fatalf("channels = %v, want 2 entries from a comma list", s.channels)
	}
	if s.channels[1] != "#alerts" {
		t.Fatalf("channels[1] = %q, want #alerts with whitespace trimmed", s.channels[1])
	}
	if !s.mentions || s.dms != 3 || s.search != "deploy" {
		t.Fatalf("surfaces not wired: mentions=%v dms=%d search=%q", s.mentions, s.dms, s.search)
	}
	if n := len(s.surfaces()); n != 5 {
		t.Fatalf("surfaces() = %d, want 5 (2 channels + mentions + dms + search)", n)
	}
}

func TestBuildQueryFallsBackToSettings(t *testing.T) {
	t.Setenv("SLACK_TOKEN", "xoxp-test")

	q, err := BuildQuery(newTestHost(nil, map[string]any{
		"channel": "from-settings",
		"dms":     4,
	}))
	if err != nil {
		t.Fatalf("BuildQuery = %v", err)
	}
	s := q.(*slackSignal)
	if len(s.channels) != 1 || s.channels[0] != "from-settings" {
		t.Fatalf("channels = %v, want the configured plugins.slack.channel", s.channels)
	}
	if s.dms != 4 {
		t.Fatalf("dms = %d, want 4 from settings", s.dms)
	}
}

func TestBuildQueryParamBeatsSetting(t *testing.T) {
	t.Setenv("SLACK_TOKEN", "xoxp-test")

	q, err := BuildQuery(newTestHost(
		map[string]string{"channel": "from-param"},
		map[string]any{"channel": "from-settings"},
	))
	if err != nil {
		t.Fatalf("BuildQuery = %v", err)
	}
	s := q.(*slackSignal)
	if len(s.channels) != 1 || s.channels[0] != "from-param" {
		t.Fatalf("channels = %v, want the param to win over the setting", s.channels)
	}
}

func TestBuildQueryWithoutTokenErrors(t *testing.T) {
	t.Setenv("SLACK_TOKEN", "")

	if _, err := BuildQuery(newTestHost(nil, nil)); err == nil {
		t.Fatal("BuildQuery succeeded with no token available")
	}
}

func TestChannelsCappedAtMax(t *testing.T) {
	var names []string
	for i := 0; i < maxChannels+5; i++ {
		names = append(names, string(rune('a'+i)))
	}
	s := NewSpec(Spec{Token: "t", Channels: names}).(*slackSignal)
	if got := len(s.channels); got != maxChannels {
		t.Fatalf("channels = %d, want the %d cap so one query cannot fan out unbounded", got, maxChannels)
	}
}

func TestBoolParam(t *testing.T) {
	cases := []struct {
		raw  string
		def  bool
		want bool
	}{
		{"", false, false},
		{"", true, true},
		{"true", false, true},
		{"1", false, true},
		{"false", true, false},
		{"0", true, false},
		{"no", true, false},
	}
	for _, c := range cases {
		p := map[string]string{}
		if c.raw != "" {
			p["mentions"] = c.raw
		}
		if got := boolParam(p, "mentions", c.def); got != c.want {
			t.Errorf("boolParam(%q, def=%v) = %v, want %v", c.raw, c.def, got, c.want)
		}
	}
}
