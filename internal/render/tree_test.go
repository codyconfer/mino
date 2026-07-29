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

	if len(rows) == 0 || !strings.Contains(rows[0].Lines[0], "flight  (3)") {
		t.Fatalf("first row should be the trunk with 3 branches, got %+v", rows[0])
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
