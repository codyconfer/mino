package ntr

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/list"

	"github.com/codyconfer/mino/internal/deck"
)

func loadedList(t *testing.T, v *recordList) *vkdeck.Model {
	t.Helper()
	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	return settle(app, v.Init())
}

func hintLabels(hints []keys.Hint) []string {
	out := make([]string, 0, len(hints))
	for _, h := range hints {
		out = append(out, h.Label)
	}
	return out
}

func hasLabel(hints []keys.Hint, label string) bool {
	for _, h := range hints {
		if h.Label == label {
			return true
		}
	}
	return false
}

func TestRecordRowsPutNewFirst(t *testing.T) {
	recs := []record{
		{Kind: kindNote, ID: 3, Title: "idea"},
		{Kind: kindNote, ID: 7, Title: "other"},
	}
	items := recordRows(80, kindNote, recs, nil)
	if len(items) != 3 {
		t.Fatalf("rows = %d, want New plus two records", len(items))
	}
	if items[0].Key != recordNewKey {
		t.Errorf("first row key = %q, want %q", items[0].Key, recordNewKey)
	}
	if !items[0].Selectable || !strings.Contains(items[0].Block, "New") {
		t.Errorf("first row = %+v, want a selectable New row", items[0])
	}
	for i, rec := range recs {
		row := items[i+1]
		if want := strconv.FormatInt(rec.ID, 10); row.Key != want {
			t.Errorf("row %d key = %q, want %q", i+1, row.Key, want)
		}
		if !strings.Contains(row.Block, rec.Title) {
			t.Errorf("row %d = %q, want it to show %q", i+1, row.Block, rec.Title)
		}
		if !row.Selectable {
			t.Errorf("row %d is not selectable", i+1)
		}
	}

	empty := recordRows(80, kindNote, nil, nil)
	if len(empty) != 2 {
		t.Fatalf("empty rows = %d, want New plus the empty state", len(empty))
	}
	if !strings.Contains(empty[1].Block, "(none yet)") {
		t.Errorf("empty state row = %q, want (none yet)", empty[1].Block)
	}
	if empty[1].Key != "" || empty[1].Selectable {
		t.Errorf("empty state row = %+v, want an unselectable keyless row", empty[1])
	}

	failed := recordRows(80, kindTask, nil, errors.New("store on fire"))
	if len(failed) != 2 {
		t.Fatalf("error rows = %d, want New plus the error", len(failed))
	}
	if !strings.Contains(failed[1].Block, "store on fire") {
		t.Errorf("error row = %q, want the store error", failed[1].Block)
	}
	if failed[1].Selectable {
		t.Error("the error row should not be selectable")
	}

	tasks := recordRows(80, kindTask, []record{
		{Kind: kindTask, ID: 1, Title: "open"},
		{Kind: kindTask, ID: 2, Title: "shut", Done: true},
	}, nil)
	if !strings.Contains(tasks[1].Block, "[ ] open") {
		t.Errorf("open task row = %q, want an unchecked mark", tasks[1].Block)
	}
	if !strings.Contains(tasks[2].Block, "[x] shut") {
		t.Errorf("done task row = %q, want a checked mark", tasks[2].Block)
	}
}

func TestListNewRowPushesBuilder(t *testing.T) {
	home := t.TempDir()
	v := newNotesList(home, "r")
	app := loadedList(t, v)

	settle(app, v.OnSelect(app, list.Item{Key: recordNewKey}))
	top, ok := app.Top().(*noteView)
	if !ok {
		t.Fatalf("top view = %T, want a *noteView", app.Top())
	}
	if got := top.Title(); got != "build note" {
		t.Errorf("pushed view title = %q, want build note", got)
	}
	if top.id != 0 {
		t.Errorf("builder id = %d, want 0", top.id)
	}
}

func TestListRowPushesEditorSeeded(t *testing.T) {
	home := t.TempDir()
	n := seedNote(t, home, "r", "idea", "the body")
	v := newNotesList(home, "r")
	app := loadedList(t, v)

	key := strconv.FormatInt(n.ID, 10)
	set := listRecords(home, "r", kindNote)
	rows := recordRows(120, kindNote, set.recs, set.err)
	var target list.Item
	for _, it := range rows {
		if it.Key == key {
			target = it
		}
	}
	if target.Key == "" {
		t.Fatalf("list rows = %v, want the seeded note under %q", rows, key)
	}
	settle(app, v.OnSelect(app, target))

	top, ok := app.Top().(*noteView)
	if !ok {
		t.Fatalf("top view = %T, want a *noteView", app.Top())
	}
	if want := "edit note #" + key; top.Title() != want {
		t.Errorf("pushed view title = %q, want %q", top.Title(), want)
	}
	if got := top.Value("title"); got != "idea" {
		t.Errorf("seeded title = %q, want idea", got)
	}
	if got := top.Value("body"); got != "the body" {
		t.Errorf("seeded body = %q, want the body", got)
	}
}

func TestListToggleMarksTaskDone(t *testing.T) {
	home := t.TempDir()
	task := seedTask(t, home, "r", "ship it", time.Time{})
	v := newTasksList(home, "r")
	app := loadedList(t, v)

	app = step(app, tea.KeyMsg{Type: tea.KeyDown})
	settle(app, cmdOf(update(app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})))

	tasks := storeTasks(t, home, "r")
	if len(tasks) != 1 || tasks[0].ID != task.ID {
		t.Fatalf("tasks = %+v, want the seeded task", tasks)
	}
	if !tasks[0].Done {
		t.Error("x did not mark the task done")
	}

	if newNotesList(home, "r").toggle != nil {
		t.Error("the notes list binds x, but a note has nothing to toggle")
	}

	rhome := t.TempDir()
	rem := seedReminder(t, rhome, "r", "ping", recordNow(t).Add(time.Hour))
	rv := newRemindersList(rhome, "r")
	rapp := loadedList(t, rv)
	rapp = step(rapp, tea.KeyMsg{Type: tea.KeyDown})
	settle(rapp, cmdOf(update(rapp, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})))

	items := storeReminders(t, rhome, "r")
	if len(items) != 1 || items[0].ID != rem.ID {
		t.Fatalf("reminders = %+v, want the seeded reminder", items)
	}
	if !items[0].Done {
		t.Error("x did not mark the reminder done")
	}
}

func TestListReloadsAfterSave(t *testing.T) {
	home := t.TempDir()
	v := newNotesList(home, "r")
	fetches := 0
	inner := v.Fetch
	v.Fetch = func() any {
		fetches++
		return inner()
	}

	app := loadedList(t, v)
	if fetches != 1 {
		t.Fatalf("fetches after load = %d, want 1", fetches)
	}

	settle(app, v.OnSelect(app, list.Item{Key: recordNewKey}))
	editor, ok := app.Top().(*noteView)
	if !ok {
		t.Fatalf("top view = %T, want a *noteView", app.Top())
	}
	setField(t, editor.Form(), "title", "fresh")
	if _, err := editor.Persist(); err != nil {
		t.Fatal(err)
	}
	if !v.stale {
		t.Fatal("saving from the editor did not mark the list stale")
	}

	settle(app, app.Pop())
	app = settle(app, cmdOf(update(app, tea.WindowSizeMsg{Width: 120, Height: 40})))
	if fetches != 2 {
		t.Fatalf("fetches after the save = %d, want 2", fetches)
	}
	if v.stale {
		t.Error("the stale flag survived the reload")
	}
	if got := app.View(); !strings.Contains(got, "fresh") {
		t.Errorf("reloaded list missing the saved note:\n%s", got)
	}
}

func TestListHintsOfferEditAndRefresh(t *testing.T) {
	notes := newNotesList("", "").Hints()
	for _, want := range []string{"edit", "refresh"} {
		if !hasLabel(notes, want) {
			t.Errorf("notes list hints missing %q: %v", want, hintLabels(notes))
		}
	}
	if hasLabel(notes, "toggle") || hasLabel(notes, "done") {
		t.Errorf("notes list offers a toggle hint: %v", hintLabels(notes))
	}

	tasks := newTasksList("", "").Hints()
	if !hasLabel(tasks, "toggle") {
		t.Errorf("tasks list hints missing toggle: %v", hintLabels(tasks))
	}
	if !hasHint(tasks, "x") {
		t.Errorf("tasks list toggle hint is not bound to x: %v", tasks)
	}

	reminders := newRemindersList("", "").Hints()
	if !hasLabel(reminders, "done") {
		t.Errorf("reminders list hints missing done: %v", hintLabels(reminders))
	}
}
