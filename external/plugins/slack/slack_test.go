package slack

import (
	"testing"
	"time"

	slackapi "github.com/slack-go/slack"
)

func TestParseSlackTS(t *testing.T) {
	got := parseSlackTS("1700000000.000200")
	want := time.Unix(1700000000, 0)
	if !got.Equal(want) {
		t.Errorf("parseSlackTS = %v, want %v", got, want)
	}

	if ts := parseSlackTS(""); !ts.IsZero() {
		t.Errorf("parseSlackTS(\"\") = %v, want zero", ts)
	}
	if ts := parseSlackTS("not-a-number"); !ts.IsZero() {
		t.Errorf("parseSlackTS(bad) = %v, want zero", ts)
	}
	if ts := parseSlackTS("1700000000"); !ts.Equal(time.Unix(1700000000, 0)) {
		t.Errorf("parseSlackTS(no frac) = %v, want %v", ts, time.Unix(1700000000, 0))
	}
}

func TestMessageToItem(t *testing.T) {
	var msg slackapi.Message
	msg.Text = "first line\nsecond line\nthird"
	msg.User = "U12345"
	msg.Timestamp = "1700000000.000200"

	item := messageToItem(msg, "eng-standup")

	if item.Kind != "message" {
		t.Errorf("Kind = %q, want message", item.Kind)
	}
	if item.Title != "first line" {
		t.Errorf("Title = %q, want %q", item.Title, "first line")
	}
	if item.Body != msg.Text {
		t.Errorf("Body = %q, want full text %q", item.Body, msg.Text)
	}
	if item.Subtitle != "#eng-standup" {
		t.Errorf("Subtitle = %q, want #eng-standup", item.Subtitle)
	}
	if item.Meta["user"] != "U12345" {
		t.Errorf("Meta[user] = %q, want U12345", item.Meta["user"])
	}
	if !item.Timestamp.Equal(time.Unix(1700000000, 0)) {
		t.Errorf("Timestamp = %v, want %v", item.Timestamp, time.Unix(1700000000, 0))
	}
	if item.URL != "" {
		t.Errorf("URL = %q, want empty", item.URL)
	}
}

func TestMessageToItemEmptyText(t *testing.T) {
	var msg slackapi.Message
	msg.Text = ""
	item := messageToItem(msg, "")
	if item.Title != "(no text)" {
		t.Errorf("Title = %q, want (no text)", item.Title)
	}
	if item.Subtitle != "" {
		t.Errorf("Subtitle = %q, want empty when channel name unknown", item.Subtitle)
	}
}

func TestMessageToItemTitleCapped(t *testing.T) {
	long := ""
	for i := 0; i < 200; i++ {
		long += "x"
	}
	var msg slackapi.Message
	msg.Text = long
	item := messageToItem(msg, "c")
	if got := len([]rune(item.Title)); got != 120 {
		t.Errorf("Title rune length = %d, want 120", got)
	}
	if item.Body != long {
		t.Errorf("Body should retain full text")
	}
}

func TestIsChannelID(t *testing.T) {
	cases := map[string]bool{
		"C123": true,
		"G999": true,
		"D001": true,
		"eng":  false,
		"#eng": false,
		"":     false,
	}
	for in, want := range cases {
		if got := isChannelID(in); got != want {
			t.Errorf("isChannelID(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestNewDefaultsLimit(t *testing.T) {
	s := New("tok", "eng", 0).(*slackSignal)
	if s.limit != defaultLimit {
		t.Errorf("limit = %d, want %d", s.limit, defaultLimit)
	}
	if got := New("tok", "eng", 10).(*slackSignal).limit; got != 10 {
		t.Errorf("limit = %d, want 10", got)
	}
	if New("tok", "eng", 0).Name() != "slack" {
		t.Errorf("Name() != slack")
	}
}
