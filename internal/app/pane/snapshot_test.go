package pane

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/internal/signals"
)

func TestSnapshotRoundTripSections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sections.json")
	want := Snapshot{
		Kind:   KindSections,
		Title:  "reviews",
		Origin: "flight:reviews",
		Sections: []signals.Section{
			{
				Signal: "github",
				Title:  "open PRs",
				Items: []signals.Item{{
					Kind:      "pr",
					Title:     "fix the thing",
					URL:       "https://example.test/1",
					Timestamp: time.Unix(1700000000, 0).UTC(),
					Meta:      map[string]string{"state": "open"},
				}},
				Meta: map[string]string{"repo": "acme/widgets"},
			},
			{Signal: "slack", Title: "mentions", Err: errors.New("token expired")},
		},
	}
	if err := WriteSnapshot(path, want); err != nil {
		t.Fatalf("WriteSnapshot: %v", err)
	}
	got, err := ReadSnapshot(path)
	if err != nil {
		t.Fatalf("ReadSnapshot: %v", err)
	}
	if got.Kind != want.Kind || got.Title != want.Title || got.Origin != want.Origin {
		t.Fatalf("header mismatch: %+v", got)
	}
	if len(got.Sections) != 2 {
		t.Fatalf("want 2 sections, got %d", len(got.Sections))
	}
	if got.Sections[0].Items[0].Meta["state"] != "open" {
		t.Errorf("item meta lost: %+v", got.Sections[0].Items[0])
	}
	if !got.Sections[0].Items[0].Timestamp.Equal(want.Sections[0].Items[0].Timestamp) {
		t.Errorf("timestamp lost: %v", got.Sections[0].Items[0].Timestamp)
	}
	if got.Sections[1].Err == nil || got.Sections[1].Err.Error() != "token expired" {
		t.Errorf("section error lost: %v", got.Sections[1].Err)
	}
}

func TestSnapshotRoundTripDetail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "detail.json")
	want := Snapshot{
		Kind:  KindDetail,
		Title: "PR #1",
		Detail: &signals.ItemDetail{
			Kind:  "pr",
			Title: "fix the thing",
			URL:   "https://example.test/1",
			Chips: []signals.Chip{
				{Label: "merged", Sev: glyph.SeverityPositive},
				{Label: "stale", Sev: glyph.SeverityWarning},
				{Label: "failing", Sev: glyph.SeverityNegative},
			},
			Rows:     [][2]string{{"author", "cody"}},
			Body:     "the body",
			Sections: []signals.DetailSection{{Title: "checks", Lines: []string{"build ok"}}},
		},
	}
	if err := WriteSnapshot(path, want); err != nil {
		t.Fatalf("WriteSnapshot: %v", err)
	}
	got, err := ReadSnapshot(path)
	if err != nil {
		t.Fatalf("ReadSnapshot: %v", err)
	}
	if got.Detail == nil {
		t.Fatal("detail lost")
	}
	if len(got.Detail.Chips) != 3 {
		t.Fatalf("want 3 chips, got %d", len(got.Detail.Chips))
	}
	for i, c := range got.Detail.Chips {
		if c.Sev != want.Detail.Chips[i].Sev {
			t.Errorf("chip %d severity: got %v want %v", i, c.Sev, want.Detail.Chips[i].Sev)
		}
	}
	if got.Detail.Rows[0] != [2]string{"author", "cody"} {
		t.Errorf("rows lost: %v", got.Detail.Rows)
	}
	if len(got.Detail.Sections) != 1 || got.Detail.Sections[0].Lines[0] != "build ok" {
		t.Errorf("detail sections lost: %+v", got.Detail.Sections)
	}
}

func TestReadSnapshotMissing(t *testing.T) {
	if _, err := ReadSnapshot(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("want error for missing snapshot")
	}
}
