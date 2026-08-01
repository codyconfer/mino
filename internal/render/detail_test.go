package render

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/codyconfer/viewkit/glyph"
	"github.com/codyconfer/viewkit/layout"

	"github.com/codyconfer/mino/internal/signals"
)

func detailRef() ItemRef {
	return ItemRef{
		Signal: "github",
		Item: signals.Item{
			Kind:      "pr",
			Title:     "Fix rate-limit backoff",
			Subtitle:  "acme/tools · In Review",
			Body:      "## Summary\nclamp the poller",
			URL:       "https://github.com/acme/tools/pull/412",
			Timestamp: time.Now().Add(-3 * time.Hour),
			Meta: map[string]string{
				"author": "cody", "state": "open", "draft": "true",
				"labels": "bug, area/signals", "status": "In Review",
				"last_comment_by": "alice",
			},
		},
	}
}

func enrichedDetail() *signals.ItemDetail {
	return &signals.ItemDetail{
		Kind:  "pr",
		Title: "Fix rate-limit backoff",
		Chips: []signals.Chip{
			{Label: "open", Sev: glyph.SeverityNeutral},
			{Label: "checks failure", Sev: glyph.SeverityNegative},
		},
		Rows: [][2]string{{"repo", "acme/tools #412"}, {"diff", "+42 −7 across 3 files"}},
		Body: "## Summary\nclamp the poller",
		Sections: []signals.DetailSection{
			{Title: "checks", Rows: [][2]string{{"lint", "failure"}}},
			{Title: "comments", Body: "### @alice · 2h ago\n\nplease clamp"},
		},
	}
}

func plain(t *testing.T, s string) string {
	t.Helper()
	return ansi.Strip(s)
}

func TestDetailPanelRendersLocalDataWithoutADetail(t *testing.T) {
	glyph.SetMode(glyph.ModeNone)
	out := plain(t, DetailPanel(layout.NewFrame(72), detailRef(), nil))

	for _, want := range []string{
		"pr",
		"Fix rate-limit backoff",
		"acme/tools · In Review",
		"author",
		"cody",
		"state",
		"open",
		"labels",
		"bug, area/signals",
		"last reply",
		"alice",
		"draft",
		"Summary",
		"clamp the poller",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("local-only detail missing %q\n%s", want, out)
		}
	}
}

func TestDetailPanelRendersEnrichedSections(t *testing.T) {
	glyph.SetMode(glyph.ModeNone)
	out := plain(t, DetailPanel(layout.NewFrame(72), detailRef(), enrichedDetail()))

	for _, want := range []string{
		"checks failure",
		"acme/tools #412",
		"+42 −7 across 3 files",
		"checks",
		"lint",
		"failure",
		"comments",
		"please clamp",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("enriched detail missing %q\n%s", want, out)
		}
	}
}

func TestDetailPanelShowsStaleCache(t *testing.T) {
	glyph.SetMode(glyph.ModeNone)
	ref := detailRef()
	ref.Meta = map[string]string{"cache": "stale", "age": "5m0s"}
	out := plain(t, DetailPanel(layout.NewFrame(72), ref, nil))
	if !strings.Contains(out, "stale 5m0s") {
		t.Errorf("want a stale cue\n%s", out)
	}

	fresh := detailRef()
	fresh.Meta = map[string]string{"cache": "hit", "age": "5s"}
	if out := plain(t, DetailPanel(layout.NewFrame(72), fresh, nil)); strings.Contains(out, "stale") {
		t.Errorf("a cache hit should not read as stale\n%s", out)
	}
}

func TestDetailPanelLinesFitTheFrame(t *testing.T) {
	glyph.SetMode(glyph.ModeNone)
	for _, width := range []int{60, 72, 100, 120} {
		f := layout.NewFrame(width)
		out := DetailPanel(f, detailRef(), enrichedDetail())
		widths := map[int]int{}
		for ln := range strings.SplitSeq(out, "\n") {
			widths[ansi.StringWidth(ln)]++
		}
		if len(widths) != 1 {
			t.Errorf("width %d produced ragged lines %v\n%s", width, widths, plain(t, out))
		}
	}
}

func TestItemIndexKeysByURL(t *testing.T) {
	sections := []signals.Section{
		{
			Signal: "github",
			Title:  "Open PRs",
			Meta:   map[string]string{"cache": "stale"},
			Items: []signals.Item{
				{Title: "one", URL: "https://github.com/acme/tools/pull/1"},
				{Title: "no url"},
				{Title: "dupe", URL: "https://github.com/acme/tools/pull/1"},
			},
		},
		{Signal: "slack", Title: "Threads", Items: []signals.Item{{Title: "two", URL: "https://slack.example/2"}}},
	}
	index := ItemIndex(sections)

	if len(index) != 2 {
		t.Fatalf("index = %d entries, want 2 (URL-less and duplicate items dropped)", len(index))
	}
	pr, ok := index["https://github.com/acme/tools/pull/1"]
	if !ok {
		t.Fatal("missing the PR entry")
	}
	if pr.Signal != "github" || pr.Item.Title != "one" {
		t.Errorf("entry = %+v, want the first item and its signal", pr)
	}
	if pr.Meta["cache"] != "stale" {
		t.Errorf("entry should carry its section meta, got %v", pr.Meta)
	}
	if index["https://slack.example/2"].Signal != "slack" {
		t.Error("slack entry lost its signal")
	}
}

func TestItemLabelAndScope(t *testing.T) {
	cases := []struct {
		name  string
		item  signals.Item
		label string
		scope string
	}{
		{"pr", signals.Item{Kind: "pr", URL: "https://github.com/acme/tools/pull/412", Subtitle: "acme/tools · In Review"}, "pr #412", "acme/tools"},
		{"issue subpage", signals.Item{Kind: "issue", URL: "https://github.com/acme/tools/issues/87/x"}, "issue #87", ""},
		{"no number", signals.Item{Kind: "pr", URL: "https://example.com/thing"}, "pr", ""},
		{"no kind", signals.Item{URL: "https://example.com"}, "detail", ""},
		{"subtitle only", signals.Item{Kind: "issue", Subtitle: "acme/tools"}, "issue", "acme/tools"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ItemLabel(c.item); got != c.label {
				t.Errorf("ItemLabel = %q, want %q", got, c.label)
			}
			if got := ItemScope(c.item); got != c.scope {
				t.Errorf("ItemScope = %q, want %q", got, c.scope)
			}
		})
	}
}

func TestItemRowsAreSelectableOnlyWithAURL(t *testing.T) {
	glyph.SetMode(glyph.ModeNone)
	rows := ItemRows(layout.NewFrame(72), []signals.Item{
		{Title: "with url", URL: "https://github.com/acme/tools/pull/1"},
		{Title: "without"},
	})
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if !rows[0].Selectable || rows[0].Key != "https://github.com/acme/tools/pull/1" {
		t.Errorf("first row = %+v, want selectable keyed by URL", rows[0])
	}
	if rows[1].Selectable {
		t.Error("a row without a URL should not be selectable")
	}
}
