package ntr

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/mino/internal/deck"
	"github.com/codyconfer/mino/internal/plugin"
	pub "github.com/codyconfer/mino/plugin"
)

func attachService(t *testing.T, on bool) {
	t.Helper()
	pub.SetServiceAttachedFunc(func() bool { return on })
	t.Cleanup(func() { pub.SetServiceAttachedFunc(plugin.ServiceAttached) })
}

func loadedBucketView(t *testing.T, v *bucketView) *vkdeck.Model {
	t.Helper()
	app := deck.New(v, deck.WithScope(testScope()))
	app = step(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	return settle(app, v.Init())
}

func TestBucketMemberRowsOfferEveryKindWhenAttached(t *testing.T) {
	attachService(t, true)
	items := bucketMemberRows(80, nil, nil)
	if len(items) != 4 {
		t.Fatalf("rows = %d, want three New rows plus the empty note", len(items))
	}
	for i, kind := range []string{kindNote, kindTask, kindReminder} {
		if want := bucketNewPrefix + kind; items[i].Key != want {
			t.Errorf("row %d key = %q, want %q", i, items[i].Key, want)
		}
		if !items[i].Selectable || !strings.Contains(items[i].Block, "New "+kind) {
			t.Errorf("row %d = %+v, want a selectable New %s row", i, items[i], kind)
		}
	}
	if !strings.Contains(items[3].Block, "(nothing filed yet)") {
		t.Errorf("last row = %q, want the empty note", items[3].Block)
	}
}

func TestBucketMemberRowsHideReminderWhenDetached(t *testing.T) {
	attachService(t, false)
	items := bucketMemberRows(80, nil, nil)
	if len(items) != 3 {
		t.Fatalf("rows = %d, want only note and task New rows plus the empty note", len(items))
	}
	for _, it := range items {
		if strings.Contains(it.Block, "New reminder") {
			t.Fatalf("row %q offered a reminder while detached", it.Block)
		}
	}
}

func TestBucketMemberRowsShowEachKind(t *testing.T) {
	attachService(t, true)
	due := time.Now().Add(time.Hour)
	recs := []record{
		{Kind: kindNote, ID: 1, Title: "a note", Body: "xy"},
		{Kind: kindTask, ID: 2, Title: "a task", Due: due},
		{Kind: kindReminder, ID: 3, Title: "a reminder", Due: due, Done: true},
	}
	items := bucketMemberRows(120, recs, nil)
	if len(items) != 6 {
		t.Fatalf("rows = %d, want three New rows plus three members", len(items))
	}
	for i, rec := range recs {
		row := items[i+3]
		if !strings.Contains(row.Block, rec.Title) {
			t.Errorf("row %d = %q, want the title", i, row.Block)
		}
		if !strings.Contains(row.Block, rec.Kind) {
			t.Errorf("row %d = %q, want the kind spelled out so a task and a reminder differ", i, row.Block)
		}
		if got, ok := row.Payload.(record); !ok || got.ID != rec.ID {
			t.Errorf("row %d payload = %+v, want the record", i, row.Payload)
		}
	}
}

func TestBucketMemberRowsRenderAnError(t *testing.T) {
	attachService(t, false)
	items := bucketMemberRows(80, nil, context.DeadlineExceeded)
	if !strings.Contains(items[len(items)-1].Block, context.DeadlineExceeded.Error()) {
		t.Fatalf("rows = %+v, want the error rendered", items)
	}
}

func TestBucketViewNewRowFilesIntoThisBucket(t *testing.T) {
	attachService(t, true)
	ctx := context.Background()
	home := t.TempDir()
	st := openStore(t, home, "r")
	b, err := st.CreateBucket(ctx, "shift", BucketKindUser, "")
	if err != nil {
		t.Fatal(err)
	}
	v := newBucketView(home, "r", b)
	app := loadedBucketView(t, v)

	app = step(app, tea.KeyMsg{Type: tea.KeyEnter})
	note, ok := app.Top().(*noteView)
	if !ok {
		t.Fatalf("enter on New note pushed %T, want a noteView", app.Top())
	}
	if note.bucket != b.ID {
		t.Fatalf("editor bucket = %d, want %d", note.bucket, b.ID)
	}
	setField(t, note.Form(), "title", "filed from the bucket")
	if _, err := note.Persist(); err != nil {
		t.Fatal(err)
	}

	recs, err := st.bucketRecords(ctx, b.ID)
	if err != nil || len(recs) != 1 || recs[0].Title != "filed from the bucket" {
		t.Fatalf("bucketRecords = %v err=%v, want the new note filed", recs, err)
	}
}

func TestBucketViewNewTaskRowFilesIntoThisBucket(t *testing.T) {
	attachService(t, true)
	ctx := context.Background()
	home := t.TempDir()
	st := openStore(t, home, "r")
	b, _ := st.CreateBucket(ctx, "shift", BucketKindUser, "")
	v := newBucketView(home, "r", b)
	app := loadedBucketView(t, v)

	app = step(app, tea.KeyMsg{Type: tea.KeyDown})
	app = step(app, tea.KeyMsg{Type: tea.KeyEnter})
	task, ok := app.Top().(*taskView)
	if !ok {
		t.Fatalf("enter on New task pushed %T, want a taskView", app.Top())
	}
	setField(t, task.Form(), "title", "a filed task")
	if _, err := task.Persist(); err != nil {
		t.Fatal(err)
	}

	recs, _ := st.bucketRecords(ctx, b.ID)
	if len(recs) != 1 || recs[0].Kind != kindTask {
		t.Fatalf("bucketRecords = %v, want one task", recs)
	}
}

func TestBucketViewEditingAMemberDoesNotRefile(t *testing.T) {
	attachService(t, true)
	ctx := context.Background()
	home := t.TempDir()
	st := openStore(t, home, "r")
	b, _ := st.CreateBucket(ctx, "shift", BucketKindUser, "")
	n, _ := st.CreateNote(ctx, "already filed", "")
	if err := st.AddMember(ctx, b.ID, n.ID, kindNote); err != nil {
		t.Fatal(err)
	}
	v := newBucketView(home, "r", b)
	app := loadedBucketView(t, v)

	for range 3 {
		app = step(app, tea.KeyMsg{Type: tea.KeyDown})
	}
	app = step(app, tea.KeyMsg{Type: tea.KeyEnter})
	note, ok := app.Top().(*noteView)
	if !ok {
		t.Fatalf("enter on a member pushed %T, want a noteView", app.Top())
	}
	if note.bucket != 0 {
		t.Fatalf("member editor bucket = %d, want 0 so a save cannot re-file", note.bucket)
	}
	setField(t, note.Form(), "title", "edited")
	if _, err := note.Persist(); err != nil {
		t.Fatal(err)
	}

	filed, _ := st.BucketsFor(ctx, n.ID)
	if len(filed) != 1 {
		t.Fatalf("BucketsFor = %v, want the single original membership", filed)
	}
}

func TestBucketViewToggleMarksATaskAndAReminder(t *testing.T) {
	attachService(t, true)
	ctx := context.Background()
	home := t.TempDir()
	st := openStore(t, home, "r")
	b, _ := st.CreateBucket(ctx, "shift", BucketKindUser, "")
	tk, _ := st.CreateTask(ctx, "a task", time.Time{})
	rm, _ := st.CreateReminder(ctx, "a reminder", time.Now().Add(time.Hour))
	for id, kind := range map[int64]string{tk.ID: kindTask, rm.ID: kindReminder} {
		if err := st.AddMember(ctx, b.ID, id, kind); err != nil {
			t.Fatal(err)
		}
	}
	v := newBucketView(home, "r", b)
	app := loadedBucketView(t, v)

	for range 3 {
		app = step(app, tea.KeyMsg{Type: tea.KeyDown})
	}
	app = step(app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	app = settle(app, nil)

	tasks, _ := st.ListTasks(ctx, true)
	if len(tasks) != 1 || !tasks[0].Done {
		t.Fatalf("tasks = %+v, want the task toggled done", tasks)
	}

	app = step(app, tea.KeyMsg{Type: tea.KeyDown})
	app = step(app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	settle(app, nil)

	open, _ := st.ListReminders(ctx, false)
	if len(open) != 0 {
		t.Fatalf("open reminders = %+v, want the reminder marked done", open)
	}
}

func TestBucketViewToggleIgnoresANote(t *testing.T) {
	attachService(t, true)
	ctx := context.Background()
	home := t.TempDir()
	st := openStore(t, home, "r")
	b, _ := st.CreateBucket(ctx, "shift", BucketKindUser, "")
	n, _ := st.CreateNote(ctx, "a note", "")
	if err := st.AddMember(ctx, b.ID, n.ID, kindNote); err != nil {
		t.Fatal(err)
	}
	v := newBucketView(home, "r", b)
	app := loadedBucketView(t, v)

	for range 3 {
		app = step(app, tea.KeyMsg{Type: tea.KeyDown})
	}
	before := app.Top()
	app = step(app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if app.Top() != before {
		t.Fatalf("toggle on a note pushed %T", app.Top())
	}
}

func TestBucketViewUnfileKeepsTheRecord(t *testing.T) {
	attachService(t, true)
	ctx := context.Background()
	home := t.TempDir()
	st := openStore(t, home, "r")
	b, _ := st.CreateBucket(ctx, "shift", BucketKindUser, "")
	n, _ := st.CreateNote(ctx, "keep me", "")
	if err := st.AddMember(ctx, b.ID, n.ID, kindNote); err != nil {
		t.Fatal(err)
	}
	v := newBucketView(home, "r", b)
	app := loadedBucketView(t, v)

	for range 3 {
		app = step(app, tea.KeyMsg{Type: tea.KeyDown})
	}
	app = step(app, tea.KeyMsg{Type: tea.KeyCtrlX})
	if got := app.View(); !strings.Contains(got, "unfile note #") {
		t.Fatalf("view = %q, want the unfile confirm", got)
	}
	if !strings.Contains(app.View(), "itself is kept") {
		t.Errorf("confirm = %q, want it to promise the record survives", app.View())
	}
	app = step(app, tea.KeyMsg{Type: tea.KeyLeft})
	app = step(app, tea.KeyMsg{Type: tea.KeyEnter})
	settle(app, nil)

	recs, _ := st.bucketRecords(ctx, b.ID)
	if len(recs) != 0 {
		t.Fatalf("bucketRecords = %v, want it unfiled", recs)
	}
	notes, _ := st.ListNotes(ctx)
	if len(notes) != 1 {
		t.Fatalf("ListNotes = %v, want the note kept", notes)
	}
}

func TestBucketViewHintsAdvertiseToggleAndUnfile(t *testing.T) {
	v := newBucketView(t.TempDir(), "r", Bucket{ID: 1, Name: "shift", Kind: BucketKindUser})
	hints := v.Hints(testScope())
	for _, want := range []string{"toggle", "unfile"} {
		if !hasLabel(hints, want) {
			t.Errorf("hints %v missing %q", hintLabels(hints), want)
		}
	}
}

func TestBucketViewContextShowsAnchorKind(t *testing.T) {
	anchored := newBucketView(t.TempDir(), "r", Bucket{ID: 1, Name: "PR #1", Kind: BucketKindItem, Anchor: "https://x/1"})
	var kinds []string
	for _, h := range anchored.Context(testScope()) {
		if h.Key == "kind" {
			kinds = append(kinds, h.Label)
		}
	}
	if len(kinds) != 1 || kinds[0] != BucketKindItem {
		t.Fatalf("context kinds = %v, want just item", kinds)
	}

	user := newBucketView(t.TempDir(), "r", Bucket{ID: 2, Name: "shift", Kind: BucketKindUser})
	for _, h := range user.Context(testScope()) {
		if h.Key == "kind" {
			t.Fatalf("user bucket context carried a kind cue: %+v", h)
		}
	}
}

func TestNewRowKind(t *testing.T) {
	for key, want := range map[string]string{
		bucketNewPrefix + kindNote:     kindNote,
		bucketNewPrefix + kindTask:     kindTask,
		bucketNewPrefix + kindReminder: kindReminder,
	} {
		got, ok := newRowKind(key)
		if !ok || got != want {
			t.Errorf("newRowKind(%q) = %q ok=%v, want %q", key, got, ok, want)
		}
	}
	for _, key := range []string{"", "7", bucketNewPrefix, "newnote"} {
		if _, ok := newRowKind(key); ok {
			t.Errorf("newRowKind(%q) claimed a kind", key)
		}
	}
}
