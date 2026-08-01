package render

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/mino/internal/render/glyph"
	"github.com/codyconfer/mino/internal/signals"
)

var refNow = time.Now()

func sampleSections() []signals.Section {
	return []signals.Section{
		{
			Signal: "github",
			Title:  "Open Pull Requests",
			Items: []signals.Item{
				{Kind: "pr", Title: "Add retry logic", Subtitle: "org/repo",
					URL: "https://github.com/org/repo/pull/1", Timestamp: refNow.Add(-2 * time.Hour),
					Meta: map[string]string{"author": "alice"}},
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
		"@alice",
		"https://github.com/org/repo/pull/1",
		"nothing to show",
		glyph.Lead(glyph.Warn()) + "token expired",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("terminal output missing %q\n---\n%s", want, out)
		}
	}
}

func TestItemLinesShowsAuthor(t *testing.T) {
	f := layout.NewFrame(80)
	th := theme.Cur()
	lines := itemLines(f, th, signals.Item{
		Kind:     "pr",
		Title:    "Add retry logic",
		Subtitle: "org/repo",
		Meta:     map[string]string{"author": "bob"},
	})
	if len(lines) == 0 {
		t.Fatal("expected at least one line")
	}
	head := ansi.Strip(lines[0])
	for _, want := range []string{"Add retry logic", "org/repo", "@bob"} {
		if !strings.Contains(head, want) {
			t.Errorf("item head missing %q\n---\n%s", want, head)
		}
	}

	noAuthor := itemLines(f, th, signals.Item{Kind: "pr", Title: "No author", Subtitle: "org/repo"})
	if len(noAuthor) == 0 {
		t.Fatal("expected at least one line")
	}
	if strings.Contains(ansi.Strip(noAuthor[0]), "@") {
		t.Errorf("item without author should not show @opener\n---\n%s", noAuthor[0])
	}
}

func TestNewUnknownFormat(t *testing.T) {
	if _, err := New(Format("xml"), "run"); err == nil {
		t.Fatal("expected error for unknown format")
	}
	if _, err := New(FormatJSON, "run"); err != nil {
		t.Fatalf("json format should be valid: %v", err)
	}
}

func TestRootLabelReplacesTheHardcodedFlight(t *testing.T) {
	secs := []signals.Section{{Signal: "github", Title: "Open PRs", Items: []signals.Item{{Title: "one"}}}}

	out := ansi.Strip(Panels(layout.NewFrame(80), "my-open-prs", secs))
	if !strings.Contains(out, "my-open-prs") {
		t.Errorf("tree root did not use the supplied label:\n%s", out)
	}
	if strings.Contains(out, "flight") {
		t.Errorf("tree root still says flight for a query run:\n%s", out)
	}

	fallback := ansi.Strip(Panels(layout.NewFrame(80), "", secs))
	if !strings.Contains(fallback, DefaultRoot) {
		t.Errorf("empty label should fall back to %q:\n%s", DefaultRoot, fallback)
	}

	items := SectionItems(layout.NewFrame(80), secs)
	if len(items) == 0 {
		t.Fatal("SectionItems returned nothing")
	}
	for _, it := range items {
		if strings.Contains(ansi.Strip(it.Block), "my-open-prs") {
			t.Errorf("SectionItems repeated the header the view already shows: %#v", it)
		}
	}
	if !strings.Contains(ansi.Strip(items[0].Block), "Open PRs") {
		t.Errorf("SectionItems should start at the first section: %#v", items[0])
	}
}
