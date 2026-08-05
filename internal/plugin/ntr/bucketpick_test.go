package ntr

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/mino/internal/deck"
	"github.com/codyconfer/mino/internal/signals"
)

const pickURL = "https://github.com/o/r/pull/7"

func pickItem() signals.Item {
	return signals.Item{Kind: "pull", Title: "fix the thing", URL: pickURL, Subtitle: "o/r · open"}
}

func loadedPicker(t *testing.T, v vkdeck.View) *vkdeck.Model {
	t.Helper()
	app := deck.New(v, deck.WithScope(testScope()))
	app = step(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	return settle(app, v.Init())
}

func TestItemTargetUsesTheURLAsAnchor(t *testing.T) {
	got := ItemTarget("github", pickItem())
	if got.Kind != BucketKindItem || got.Anchor != pickURL {
		t.Fatalf("target = %+v, want an item anchored to the URL", got)
	}
	if got.Name != "fix the thing" || got.Title != "fix the thing" {
		t.Errorf("target name/title = %q/%q, want the item title", got.Name, got.Title)
	}
	if got.Body != pickURL {
		t.Errorf("target body = %q, want the URL so a note keeps the link", got.Body)
	}
}

func TestItemTargetFallsBackToTheSignalName(t *testing.T) {
	got := ItemTarget("github", signals.Item{URL: pickURL})
	if got.Name != "github" {
		t.Fatalf("name = %q, want the signal name when the item has no title", got.Name)
	}
}

func TestRunTargetAnchorsOnTheRunID(t *testing.T) {
	got := RunTarget(184, "flight", "mino-ci")
	if got.Kind != BucketKindRun || got.Anchor != RunAnchor(184) {
		t.Fatalf("target = %+v, want a run anchored to run:184", got)
	}
	if !strings.Contains(got.Name, "mino-ci") || !strings.Contains(got.Name, "#184") {
		t.Errorf("name = %q, want the flight name and run id", got.Name)
	}
	if got.Body != "" {
		t.Errorf("body = %q, want empty for a run", got.Body)
	}
}

func TestNewBucketPickerRefusesAnUnanchorableItem(t *testing.T) {
	v := NewBucketPicker(t.TempDir(), "r", ItemTarget("ntr", signals.Item{Title: "no url"}), nil, testScheme())
	if _, ok := v.(*bucketPicker); ok {
		t.Fatal("picker opened for an item with no URL")
	}
	if got := v.Title(); got != "nothing to anchor" {
		t.Fatalf("title = %q, want the nothing-to-anchor message", got)
	}
}

func TestPickerAnchorRowReportsNotFiledYet(t *testing.T) {
	attachService(t, true)
	home := t.TempDir()
	openStore(t, home, "r")
	v := NewBucketPicker(home, "r", ItemTarget("github", pickItem()), nil, testScheme())
	app := loadedPicker(t, v)

	if got := app.View(); !strings.Contains(got, "not filed yet") {
		t.Fatalf("view = %q, want the anchor row to say nothing is filed", got)
	}
	if !strings.Contains(app.View(), "This item") {
		t.Errorf("view = %q, want a This item row", app.View())
	}
}

func TestPickerAnchorRowCountsExistingRecords(t *testing.T) {
	attachService(t, true)
	ctx := context.Background()
	home := t.TempDir()
	st := openStore(t, home, "r")
	b, _ := st.EnsureAnchorBucket(ctx, BucketKindItem, pickURL, "PR #7")
	n, _ := st.CreateNote(ctx, "earlier", "")
	if err := st.AddMember(ctx, b.ID, n.ID, kindNote); err != nil {
		t.Fatal(err)
	}

	v := NewBucketPicker(home, "r", ItemTarget("github", pickItem()), nil, testScheme())
	app := loadedPicker(t, v)
	if got := app.View(); !strings.Contains(got, "1 record already filed") {
		t.Fatalf("view = %q, want the existing count", got)
	}
	if strings.Count(app.View(), "PR #7") > 0 {
		t.Errorf("view = %q, want the anchor bucket not repeated as a plain row", app.View())
	}
}

func TestPickerAnchorRowFilesIntoTheAnchorBucket(t *testing.T) {
	attachService(t, true)
	ctx := context.Background()
	home := t.TempDir()
	st := openStore(t, home, "r")
	v := NewBucketPicker(home, "r", ItemTarget("github", pickItem()), nil, testScheme())
	app := loadedPicker(t, v)

	app = step(app, tea.KeyMsg{Type: tea.KeyDown})
	app = step(app, tea.KeyMsg{Type: tea.KeyEnter})
	app = settle(app, nil)
	if _, ok := app.Top().(*vkdeck.Menu); !ok {
		t.Fatalf("anchor row pushed %T, want the kind menu", app.Top())
	}

	app = step(app, tea.KeyMsg{Type: tea.KeyEnter})
	app = settle(app, nil)
	note, ok := app.Top().(*noteView)
	if !ok {
		t.Fatalf("kind menu pushed %T, want a noteView", app.Top())
	}
	if note.Value("title") != "fix the thing" {
		t.Errorf("seeded title = %q, want the item title", note.Value("title"))
	}
	if _, err := note.Persist(); err != nil {
		t.Fatal(err)
	}

	counts, err := st.AnchorCounts(ctx, BucketKindItem, []string{pickURL})
	if err != nil || counts[pickURL] != 1 {
		t.Fatalf("AnchorCounts = %v err=%v, want the item to show one filed record", counts, err)
	}
	if len(note.extra) != 0 {
		t.Errorf("extra = %v, want none when the anchor bucket is the chosen one", note.extra)
	}
}

func TestPickerUserBucketAlsoFilesAgainstTheAnchor(t *testing.T) {
	attachService(t, true)
	ctx := context.Background()
	home := t.TempDir()
	st := openStore(t, home, "r")
	if _, err := st.CreateBucket(ctx, "escalations", BucketKindUser, ""); err != nil {
		t.Fatal(err)
	}
	v := NewBucketPicker(home, "r", ItemTarget("github", pickItem()), nil, testScheme())
	app := loadedPicker(t, v)

	for range 2 {
		app = step(app, tea.KeyMsg{Type: tea.KeyDown})
	}
	app = step(app, tea.KeyMsg{Type: tea.KeyEnter})
	app = settle(app, nil)
	app = step(app, tea.KeyMsg{Type: tea.KeyEnter})
	app = settle(app, nil)
	note, ok := app.Top().(*noteView)
	if !ok {
		t.Fatalf("pushed %T, want a noteView", app.Top())
	}
	if len(note.extra) != 1 {
		t.Fatalf("extra = %v, want the anchor bucket added alongside the user bucket", note.extra)
	}
	if _, err := note.Persist(); err != nil {
		t.Fatal(err)
	}

	filed, _ := st.BucketsFor(ctx, note.id)
	if len(filed) != 2 {
		t.Fatalf("BucketsFor = %v, want both the user bucket and the anchor bucket", filed)
	}
	counts, _ := st.AnchorCounts(ctx, BucketKindItem, []string{pickURL})
	if counts[pickURL] != 1 {
		t.Fatalf("AnchorCounts = %v, want the item cue to stay truthful", counts)
	}
}

func TestPickerNewBucketRowCreatesThenFiles(t *testing.T) {
	attachService(t, true)
	ctx := context.Background()
	home := t.TempDir()
	st := openStore(t, home, "r")
	v := NewBucketPicker(home, "r", ItemTarget("github", pickItem()), nil, testScheme())
	app := loadedPicker(t, v)

	app = step(app, tea.KeyMsg{Type: tea.KeyEnter})
	if _, ok := app.Top().(*vkdeck.FormView); !ok {
		t.Fatalf("New bucket pushed %T, want a form", app.Top())
	}
	app = step(app, tea.KeyMsg{Type: tea.KeyCtrlS})
	app = settle(app, nil)
	if _, ok := app.Top().(*vkdeck.Menu); !ok {
		t.Fatalf("after naming, top = %T, want the kind menu", app.Top())
	}

	bs, _ := st.ListBuckets(ctx)
	if len(bs) != 1 || bs[0].Name != "fix the thing" {
		t.Fatalf("ListBuckets = %v, want one seeded from the item title", bs)
	}
	if bs[0].Kind != BucketKindUser {
		t.Errorf("kind = %q, want a user bucket", bs[0].Kind)
	}
}

func TestPickerKindMenuHidesReminderWhenDetached(t *testing.T) {
	attachService(t, false)
	home := t.TempDir()
	openStore(t, home, "r")
	v := NewBucketPicker(home, "r", ItemTarget("github", pickItem()), nil, testScheme())
	app := loadedPicker(t, v)

	app = step(app, tea.KeyMsg{Type: tea.KeyDown})
	app = step(app, tea.KeyMsg{Type: tea.KeyEnter})
	app = settle(app, nil)
	got := app.View()
	if strings.Contains(got, "Reminder") {
		t.Fatalf("kind menu = %q, want no reminder while detached", got)
	}
	for _, want := range []string{"Note", "Task"} {
		if !strings.Contains(got, want) {
			t.Fatalf("kind menu = %q, missing %q", got, want)
		}
	}
}

func TestPickerRunTargetFilesAgainstTheRun(t *testing.T) {
	attachService(t, true)
	ctx := context.Background()
	home := t.TempDir()
	st := openStore(t, home, "r")
	v := NewBucketPicker(home, "r", RunTarget(184, "flight", "mino-ci"), nil, testScheme())
	app := loadedPicker(t, v)

	if got := app.View(); !strings.Contains(got, "This run") {
		t.Fatalf("view = %q, want a This run row", got)
	}
	app = step(app, tea.KeyMsg{Type: tea.KeyDown})
	app = step(app, tea.KeyMsg{Type: tea.KeyEnter})
	app = settle(app, nil)
	app = step(app, tea.KeyMsg{Type: tea.KeyDown})
	app = step(app, tea.KeyMsg{Type: tea.KeyEnter})
	app = settle(app, nil)
	task, ok := app.Top().(*taskView)
	if !ok {
		t.Fatalf("pushed %T, want a taskView", app.Top())
	}
	if _, err := task.Persist(); err != nil {
		t.Fatal(err)
	}

	n, err := RunFiledCount(home, "r", 184)
	if err != nil || n != 1 {
		t.Fatalf("RunFiledCount = %d err=%v, want 1", n, err)
	}
	counts, _ := st.AnchorCounts(ctx, BucketKindItem, []string{RunAnchor(184)})
	if len(counts) != 0 {
		t.Fatalf("AnchorCounts(item) = %v, want a run anchor kept out of the item cue", counts)
	}
}

func TestPickerReportsAStoreError(t *testing.T) {
	items := pickRows(80, ItemTarget("github", pickItem()), pickSet{err: context.Canceled})
	if len(items) != 2 || !strings.Contains(items[1].Block, "canceled") {
		t.Fatalf("rows = %+v, want the error shown instead of an anchor row", items)
	}
}

func TestPickerDirtyCallbackReachesTheEditor(t *testing.T) {
	attachService(t, true)
	home := t.TempDir()
	openStore(t, home, "r")
	var marked bool
	v := NewBucketPicker(home, "r", ItemTarget("github", pickItem()), func() { marked = true }, testScheme())
	app := loadedPicker(t, v)

	app = step(app, tea.KeyMsg{Type: tea.KeyDown})
	app = step(app, tea.KeyMsg{Type: tea.KeyEnter})
	app = settle(app, nil)
	app = step(app, tea.KeyMsg{Type: tea.KeyEnter})
	app = settle(app, nil)
	note, ok := app.Top().(*noteView)
	if !ok {
		t.Fatalf("pushed %T, want a noteView", app.Top())
	}
	if _, err := note.Persist(); err != nil {
		t.Fatal(err)
	}
	if !marked {
		t.Fatal("saving from the picker did not mark the caller stale")
	}
}
