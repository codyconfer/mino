package slack

import "testing"

func TestPermalinkRoundTrip(t *testing.T) {
	ref := msgRef{ChannelID: "C0123ABC", TS: "1700000000.123456"}
	url := permalink("myorg.slack.com", ref)
	want := "https://myorg.slack.com/archives/C0123ABC/p1700000000123456"
	if url != want {
		t.Fatalf("permalink = %q, want %q", url, want)
	}

	got, ok := parsePermalink(url)
	if !ok {
		t.Fatalf("parsePermalink(%q) failed", url)
	}
	if got.ChannelID != ref.ChannelID || got.TS != ref.TS {
		t.Fatalf("round trip = %+v, want %+v", got, ref)
	}
}

func TestPermalinkThreadReply(t *testing.T) {
	ref := msgRef{ChannelID: "C1", TS: "1700000009.000002", ThreadTS: "1700000000.000001"}
	url := permalink("myorg.slack.com", ref)

	got, ok := parsePermalink(url)
	if !ok {
		t.Fatalf("parsePermalink(%q) failed", url)
	}
	if got.TS != ref.TS {
		t.Fatalf("TS = %q, want the reply ts %q", got.TS, ref.TS)
	}
	if got.ThreadTS != ref.ThreadTS {
		t.Fatalf("ThreadTS = %q, want %q so Detail fetches the thread root", got.ThreadTS, ref.ThreadTS)
	}
	if got.root() != ref.ThreadTS {
		t.Fatalf("root() = %q, want the parent ts %q", got.root(), ref.ThreadTS)
	}
}

func TestPermalinkEmptyWhenIncomplete(t *testing.T) {
	cases := []struct {
		name string
		host string
		ref  msgRef
	}{
		{"no host", "", msgRef{ChannelID: "C1", TS: "1700000000.000001"}},
		{"no channel", "h", msgRef{TS: "1700000000.000001"}},
		{"no ts", "h", msgRef{ChannelID: "C1"}},
	}
	for _, c := range cases {
		if got := permalink(c.host, c.ref); got != "" {
			t.Errorf("%s: permalink = %q, want empty rather than a link that 404s", c.name, got)
		}
	}
}

func TestRootIsSelfWhenNotThreaded(t *testing.T) {
	ref := msgRef{ChannelID: "C1", TS: "1700000000.000001"}
	if ref.root() != ref.TS {
		t.Fatalf("root() = %q, want the message's own ts", ref.root())
	}
}

func TestParsePermalinkRejects(t *testing.T) {
	bad := []string{
		"",
		"https://myorg.slack.com/",
		"https://myorg.slack.com/archives/",
		"https://myorg.slack.com/archives/C1",
		"https://myorg.slack.com/archives/eng/p1700000000123456",
		"https://myorg.slack.com/archives/C1/p12345",
		"https://myorg.slack.com/archives/C1/pnotdigits",
		"https://github.com/owner/repo/pull/1",
	}
	for _, in := range bad {
		if ref, ok := parsePermalink(in); ok {
			t.Errorf("parsePermalink(%q) = %+v, want rejected", in, ref)
		}
	}
}

func TestParsePermalinkAcceptsDottedTS(t *testing.T) {
	got, ok := parsePermalink("myorg.slack.com/archives/C1/p1700000000.123456")
	if !ok {
		t.Fatal("a hand-pasted dotted ts should still parse")
	}
	if got.TS != "1700000000.123456" {
		t.Fatalf("TS = %q", got.TS)
	}
}

func TestTSPathSegment(t *testing.T) {
	if got := tsPathSegment("1700000000.123456"); got != "p1700000000123456" {
		t.Fatalf("tsPathSegment = %q", got)
	}
}
