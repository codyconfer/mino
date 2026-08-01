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

func TestSectionResultsCountIncludesErrSections(t *testing.T) {
	r := SectionResults{Sections: []signals.Section{
		{Signal: "gmail", Err: errString("token expired")},
		{Signal: "slack"},
		{Signal: "github", Items: []signals.Item{{Title: "a"}, {Title: "b"}}},
	}}
	if got := r.Count(); got != 3 {
		t.Fatalf("Count() = %d, want 3 (one error leaf + two items)", got)
	}
}

func TestSectionResultsErroredCountsErrSections(t *testing.T) {
	r := SectionResults{Sections: []signals.Section{
		{Signal: "gmail", Err: errString("token expired")},
		{Signal: "slack"},
		{Signal: "github", Items: []signals.Item{{Title: "a"}, {Title: "b"}}},
	}}
	if got := r.Errored(); got != 1 {
		t.Fatalf("Errored() = %d, want 1", got)
	}
	if got := (SectionResults{}).Errored(); got != 0 {
		t.Fatalf("Errored() on empty = %d, want 0", got)
	}
}

func TestItemLinesShowsAuthor(t *testing.T) {
	f := layout.NewFrame(80)
	th := theme.Default()
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

func TestWorkflowResultInProgressUsesSpinnerGlyph(t *testing.T) {
	previous := glyph.CurrentMode()
	glyph.SetMode(glyph.ModeNone)
	t.Cleanup(func() { glyph.SetMode(previous) })

	f := layout.NewFrame(80)
	th := theme.Default()
	inProgress := signals.Item{
		Kind:  "workflow",
		Title: "CI #42",
		Meta:  map[string]string{"status": "in_progress", "state": "in progress"},
	}
	head := ansi.Strip(itemLines(f, th, inProgress)[0])
	if !strings.HasPrefix(head, glyph.Lead("|")+"CI #42") {
		t.Fatalf("in-progress workflow head = %q, want spinner glyph", head)
	}

	completed := inProgress
	completed.Meta = map[string]string{"status": "completed", "conclusion": "success", "state": "success"}
	head = ansi.Strip(itemLines(f, th, completed)[0])
	if !strings.HasPrefix(head, glyph.Lead(glyph.Check())+"CI #42") {
		t.Fatalf("completed workflow head = %q, want success glyph", head)
	}
}

func TestSectionResultsAdvanceWorkflowSpinner(t *testing.T) {
	previous := glyph.CurrentMode()
	glyph.SetMode(glyph.ModeNone)
	t.Cleanup(func() { glyph.SetMode(previous) })

	r := &SectionResults{Sections: []signals.Section{{
		Signal: "github",
		Title:  "Workflows",
		Items: []signals.Item{{
			Kind:  "workflow",
			Title: "CI #42",
			Meta:  map[string]string{"status": "in_progress"},
		}},
	}}}
	before := r.Items(layout.NewFrame(80))[1].Block
	if !r.Advance() {
		t.Fatal("in-progress workflow did not request another frame")
	}
	after := r.Items(layout.NewFrame(80))[1].Block
	if before == after {
		t.Fatalf("workflow spinner stayed frozen: %q", after)
	}

	r.Sections[0].Items[0].Meta = map[string]string{"status": "completed", "conclusion": "success"}
	if r.Advance() {
		t.Fatal("completed workflow requested another frame")
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
