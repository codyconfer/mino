package render

import (
	"fmt"
	"strings"
	"testing"

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
