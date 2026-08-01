package views

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/codyconfer/mino/internal/app/pane"
	"github.com/codyconfer/mino/internal/signals"
)

// The deck chrome already draws the snapshot title as a breadcrumb, so the body
// must not repeat it as a tree trunk.
func TestSnapshotBodyDoesNotRepeatTheChromeTitle(t *testing.T) {
	const title = "flight: triage-check"
	path := filepath.Join(t.TempDir(), "pane.json")
	snap := pane.Snapshot{
		Kind:   pane.KindSections,
		Title:  title,
		Origin: "flight:triage-check",
		Sections: []signals.Section{
			{Signal: "github", Title: "Escalations · Incoming", Items: []signals.Item{
				{Title: "a broken dashboard", URL: "u1"},
			}},
		},
	}
	if err := pane.WriteSnapshot(path, snap); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	v := NewSnapshotView(path)
	msg := v.loadCmd()()
	loaded, ok := msg.(snapshotLoadedMsg)
	if !ok {
		t.Fatalf("loadCmd returned %T, want snapshotLoadedMsg", msg)
	}
	if loaded.err != nil {
		t.Fatalf("load snapshot: %v", loaded.err)
	}
	v.Update(nil, loaded)

	if v.Title() != title {
		t.Fatalf("chrome title = %q, want %q", v.Title(), title)
	}
	body := ansi.Strip(v.render(80))
	if strings.Contains(body, title) {
		t.Errorf("body repeated the chrome title:\n%s", body)
	}
	if !strings.Contains(body, "Escalations · Incoming") {
		t.Errorf("body should start at the first section head:\n%s", body)
	}
}
