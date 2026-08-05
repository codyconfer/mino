package render

import (
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/mino/internal/signals"
)

func filedItem(n string) signals.Item {
	it := signals.Item{
		Kind:      "pull",
		Title:     "fix the thing",
		URL:       "https://github.com/o/r/pull/7",
		Timestamp: time.Now().Add(-time.Hour),
	}
	if n != "" {
		it.Meta = map[string]string{signals.MetaFiled: n}
	}
	return it
}

func TestFiledChipRendersACount(t *testing.T) {
	f := layout.ScreenFrame(100)
	lines := itemLines(f, theme.Default(), filedItem("2"))
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "notes 2") {
		t.Fatalf("lines = %q, want a notes 2 chip", joined)
	}
}

func TestFiledChipAbsentWhenUnset(t *testing.T) {
	f := layout.ScreenFrame(100)
	for _, n := range []string{"", "0", "  "} {
		lines := itemLines(f, theme.Default(), filedItem(n))
		joined := strings.Join(lines, "\n")
		if strings.Contains(joined, "notes") {
			t.Errorf("filed=%q rendered a chip: %q", n, joined)
		}
	}
}

func TestFiledChipKeepsTheTimestamp(t *testing.T) {
	f := layout.ScreenFrame(100)
	bare := strings.Join(itemLines(f, theme.Default(), filedItem("")), "\n")
	withChip := strings.Join(itemLines(f, theme.Default(), filedItem("3")), "\n")
	if !strings.Contains(bare, "ago") {
		t.Fatalf("baseline = %q, want a relative timestamp to compare against", bare)
	}
	if !strings.Contains(withChip, "ago") {
		t.Fatalf("lines = %q, want the timestamp kept alongside the chip", withChip)
	}
	if !strings.Contains(withChip, "notes 3") {
		t.Fatalf("lines = %q, want the chip", withChip)
	}
}

func TestDetailGutterGainsANotesRow(t *testing.T) {
	ref := ItemRef{Signal: "github", Item: filedItem("2")}
	rows := localRows(ref)
	var found string
	for _, r := range rows {
		if r[0] == "notes" {
			found = r[1]
		}
	}
	if found != "2" {
		t.Fatalf("rows = %v, want a notes row of 2", rows)
	}
}

func TestDetailGutterOmitsAZeroNotesRow(t *testing.T) {
	for _, n := range []string{"", "0"} {
		rows := localRows(ItemRef{Signal: "github", Item: filedItem(n)})
		for _, r := range rows {
			if r[0] == "notes" {
				t.Errorf("filed=%q produced a notes row: %v", n, rows)
			}
		}
	}
}
