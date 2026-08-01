package views

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/codyconfer/viewkit/layout"

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
	body := ansi.Strip(v.render(layout.Frame{Width: 80}))
	if strings.Contains(body, title) {
		t.Errorf("body repeated the chrome title:\n%s", body)
	}
	if !strings.Contains(body, "Escalations · Incoming") {
		t.Errorf("body should start at the first section head:\n%s", body)
	}
}

func TestSnapshotAnimatesInProgressWorkflow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pane.json")
	snap := pane.Snapshot{
		Kind:   pane.KindSections,
		Title:  "flight: ci",
		Origin: "flight:ci",
		Sections: []signals.Section{
			{Signal: "github", Title: "Workflows", Items: []signals.Item{{
				Kind:  "workflow",
				Title: "CI #42",
				URL:   "https://github.com/acme/tools/actions/runs/42",
				Meta:  map[string]string{"status": "in_progress", "state": "in progress"},
			}}},
		},
	}
	if err := pane.WriteSnapshot(path, snap); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	v := NewSnapshotView(path)
	if cmd := v.Update(nil, v.loadCmd()().(snapshotLoadedMsg)); cmd == nil {
		t.Fatal("in-progress workflow did not start the animation tick")
	}
	f := layout.Frame{Width: 80}
	before := ansi.Strip(v.render(f))
	if cmd := v.Update(nil, snapshotAnimationMsg{}); cmd == nil {
		t.Fatal("in-progress workflow did not continue the animation tick")
	}
	if after := ansi.Strip(v.render(f)); after == before {
		t.Fatalf("snapshot spinner stayed frozen:\n%s", after)
	}

	v.snap.Sections[0].Items[0].Meta = map[string]string{"status": "completed", "conclusion": "success"}
	if cmd := v.Update(nil, snapshotAnimationMsg{}); cmd != nil {
		t.Error("completed workflow kept the animation tick alive")
	}
}

func TestSnapshotSettledSectionsRenderNoSpinnerFrame(t *testing.T) {
	v := NewSnapshotView(filepath.Join(t.TempDir(), "pane.json"))
	v.snap = pane.Snapshot{Kind: pane.KindSections, Sections: []signals.Section{
		{Signal: "github", Title: "Workflows", Items: []signals.Item{
			{Kind: "workflow", Title: "CI #41", Meta: map[string]string{"status": "completed", "conclusion": "success"}},
		}},
	}}
	if got := v.animFrame(); got != -1 {
		t.Fatalf("animFrame = %d, want -1 while nothing is in progress", got)
	}
}
