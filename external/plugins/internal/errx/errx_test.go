package errx

import (
	"errors"
	"strings"
	"testing"
)

func TestErrorRendersEachHintOnce(t *testing.T) {
	inner := New("no Slack token available").WithHint("export a user token or run `munin login slack`")
	outer := Wrap(inner, "slack authentication").WithHint("set SLACK_TOKEN")

	got := outer.Error()
	want := "slack authentication: no Slack token available\n" +
		"hint: set SLACK_TOKEN\n" +
		"hint: export a user token or run `munin login slack`"
	if got != want {
		t.Fatalf("Error() =\n%s\nwant\n%s", got, want)
	}
	if strings.Count(got, "hint:") != 2 {
		t.Fatalf("hints repeated in %q", got)
	}
}

func TestErrorUnwrapsToTheCause(t *testing.T) {
	sentinel := errors.New("boom")
	if !errors.Is(Wrap(sentinel, "context"), sentinel) {
		t.Fatal("Wrap must keep the cause reachable through errors.Is")
	}
}
