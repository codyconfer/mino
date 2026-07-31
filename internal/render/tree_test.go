package render

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/codyconfer/viewkit/layout"

	"github.com/codyconfer/munin/internal/render/glyph"
	"github.com/codyconfer/munin/internal/signals"
)

func TestFlightTreeStructure(t *testing.T) {
	glyph.SetMode(glyph.ModeNone)
	secs := []signals.Section{
		{Signal: "github", Items: []signals.Item{{Title: "a", URL: "u1"}, {Title: "b"}}},
		{Signal: "cal"},
		{Signal: "issues", Err: fmt.Errorf("boom")},
	}
	rows := FlightTree(layout.NewFrame(80), "flight", secs)

	if len(rows) == 0 || !strings.Contains(rows[0].Lines[0], "flight") {
		t.Fatalf("first row should be the trunk, got %+v", rows[0])
	}

	var selectable, empty, errBranch bool
	for _, r := range rows {
		if r.Key == "u1" && r.Selectable {
			selectable = true
		}
		joined := strings.Join(r.Lines, "\n")
		if strings.Contains(joined, "nothing to show") {
			empty = true
		}
		if strings.Contains(joined, "(!)") {
			errBranch = true
		}
	}
	if !selectable {
		t.Error("expected a selectable leaf with Key=u1")
	}
	if !empty {
		t.Error("expected an empty branch to render 'nothing to show'")
	}
	if !errBranch {
		t.Error("expected the error section to render a (!) count")
	}
}

func TestFlightTreeGapStemContinuesConnectors(t *testing.T) {
	glyph.SetMode(glyph.ModeNone)
	secs := []signals.Section{
		{Signal: "github", Items: []signals.Item{
			{Title: "a", URL: "u1"},
			{Title: "b", URL: "u2"},
		}},
		{Signal: "cal", Items: []signals.Item{{Title: "c", URL: "u3"}}},
	}
	rows := FlightTree(layout.NewFrame(80), "flight", secs)

	if rows[0].GapStem != "" {
		t.Fatalf("trunk GapStem = %q, want empty", rows[0].GapStem)
	}

	var midLeaf, lastLeaf, sectionStem string
	for _, r := range rows {
		joined := ansi.Strip(strings.Join(r.Lines, "\n"))
		switch {
		case r.Key == "u1":
			midLeaf = ansi.Strip(r.GapStem)
		case r.Key == "u2":
			lastLeaf = ansi.Strip(r.GapStem)
		case strings.Contains(joined, "github") && r.Key == "":
			sectionStem = ansi.Strip(r.GapStem)
		}
	}
	if midLeaf != "|  |  " {
		t.Fatalf("mid leaf GapStem = %q, want %q", midLeaf, "|  |  ")
	}
	if lastLeaf != "|     " {
		t.Fatalf("last leaf in section GapStem = %q, want %q", lastLeaf, "|     ")
	}
	if sectionStem != "|  " {
		t.Fatalf("section GapStem = %q, want %q", sectionStem, "|  ")
	}

	items := SectionItems(layout.NewFrame(80), "run", secs)
	var found bool
	for _, it := range items {
		if it.Key == "u1" {
			found = true
			if got := ansi.Strip(it.GapStem); got != "|  |  " {
				t.Fatalf("SectionItems GapStem = %q, want %q", got, "|  |  ")
			}
		}
	}
	if !found {
		t.Fatal("expected SectionItems to include u1")
	}
}

func TestFlightTreeCuesTruncatedSections(t *testing.T) {
	glyph.SetMode(glyph.ModeNone)
	cases := []struct {
		name string
		meta map[string]string
		want string
	}{
		{
			name: "signal truncation with a remainder",
			meta: map[string]string{
				"shown":            "2",
				"total":            "97",
				"more":             "95",
				"truncated":        "true",
				"truncated_reason": "github's search backend timed out; these results are incomplete",
			},
			want: "(truncated, +95 more)",
		},
		{
			name: "signal truncation without a remainder",
			meta: map[string]string{"shown": "2", "truncated": "true"},
			want: "(truncated)",
		},
		{
			name: "serve frame truncation",
			meta: map[string]string{"munin.truncated": "1300000 byte event exceeds the 1048576 byte frame limit"},
			want: "(truncated)",
		},
		{
			name: "stale and truncated together",
			meta: map[string]string{"cache": "stale", "age": "2m0s", "truncated": "true"},
			want: "(truncated)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			secs := []signals.Section{{
				Signal: "github",
				Title:  "Review Requests",
				Items:  []signals.Item{{Title: "a", URL: "u1"}, {Title: "b", URL: "u2"}},
				Meta:   tc.meta,
			}}
			var branch string
			for _, r := range FlightTree(layout.NewFrame(80), "flight", secs) {
				line := ansi.Strip(strings.Join(r.Lines, "\n"))
				if strings.Contains(line, "Review Requests") {
					branch = line
				}
			}
			if branch == "" {
				t.Fatal("no section branch rendered")
			}
			if !strings.Contains(branch, tc.want) {
				t.Errorf("branch = %q, want it to carry %q: a list cut short by a backend timeout or a frame "+
					"limit otherwise reads as a complete short list", branch, tc.want)
			}
			if tc.meta["cache"] == "stale" && !strings.Contains(branch, "stale") {
				t.Errorf("branch = %q, want the stale cue kept alongside the truncation cue", branch)
			}
		})
	}
}

func TestFlightTreeLeavesCompleteSectionsUncued(t *testing.T) {
	glyph.SetMode(glyph.ModeNone)
	for _, meta := range []map[string]string{
		nil,
		{"shown": "2", "total": "2"},
		{"shown": "2", "total": "97", "more": "95"},
		{"truncated": "false"},
	} {
		secs := []signals.Section{{
			Signal: "github",
			Title:  "Review Requests",
			Items:  []signals.Item{{Title: "a", URL: "u1"}},
			Meta:   meta,
		}}
		for _, r := range FlightTree(layout.NewFrame(80), "flight", secs) {
			line := ansi.Strip(strings.Join(r.Lines, "\n"))
			if strings.Contains(line, "truncated") {
				t.Errorf("meta %v rendered %q, want no truncation cue", meta, line)
			}
		}
	}
}
