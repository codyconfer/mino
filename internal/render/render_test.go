package render

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/munin/internal/signals"
)

var refNow = time.Now()

func sampleSections() []signals.Section {
	return []signals.Section{
		{
			Signal: "github",
			Title:  "Open Pull Requests",
			Items: []signals.Item{
				{Kind: "pr", Title: "Add retry logic", Subtitle: "org/repo",
					URL: "https://github.com/org/repo/pull/1", Timestamp: refNow.Add(-2 * time.Hour)},
			},
		},
		{Signal: "slack", Title: "#eng", Items: nil},
		{Signal: "gmail", Title: "Gmail", Err: errString("token expired")},
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestJSONRenderer(t *testing.T) {
	var buf bytes.Buffer
	if err := (&JSONRenderer{}).Render(&buf, sampleSections()); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		`"signal": "github"`,
		`"title": "Open Pull Requests"`,
		`"kind": "pr"`,
		`"error": "token expired"`,
		`"items": []`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("json output missing %q\n---\n%s", want, out)
		}
	}
}

func TestTerminalRendererPlain(t *testing.T) {

	var buf bytes.Buffer
	if err := (&TerminalRenderer{}).Render(&buf, sampleSections()); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "\x1b[") {
		t.Errorf("terminal output should have no ANSI codes when not a TTY:\n%q", out)
	}
	for _, want := range []string{
		"Open Pull Requests  (1)",
		"Add retry logic",
		"org/repo",
		"https://github.com/org/repo/pull/1",
		"nothing to show",
		"⚠ token expired",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("terminal output missing %q\n---\n%s", want, out)
		}
	}
}

func TestNewUnknownFormat(t *testing.T) {
	if _, err := New(Format("xml")); err == nil {
		t.Fatal("expected error for unknown format")
	}
	if _, err := New(FormatJSON); err != nil {
		t.Fatalf("json format should be valid: %v", err)
	}
}
