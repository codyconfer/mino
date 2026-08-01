package ntr

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/codyconfer/viewkit/forms"
	"github.com/codyconfer/viewkit/keys"

	"github.com/codyconfer/mino/internal/deck"
)

func setField(t *testing.T, fm *forms.Form, key, val string) {
	t.Helper()
	for i := range fm.Fields {
		if fm.Fields[i].Key == key {
			fm.Fields[i].Text = val
			return
		}
	}
	t.Fatalf("form has no field %q", key)
}

func setToggle(t *testing.T, fm *forms.Form, key string, on bool) {
	t.Helper()
	for i := range fm.Fields {
		if fm.Fields[i].Key == key {
			fm.Fields[i].On = on
			return
		}
	}
	t.Fatalf("form has no field %q", key)
}

func fieldKeys(fields []forms.Field) []string {
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		out = append(out, f.Key)
	}
	return out
}

func fieldKinds(fields []forms.Field) []forms.FieldKind {
	out := make([]forms.FieldKind, 0, len(fields))
	for _, f := range fields {
		out = append(out, f.Kind)
	}
	return out
}

func hasHint(hints []keys.Hint, glyph string) bool {
	for _, h := range hints {
		if h.Key == glyph {
			return true
		}
	}
	return false
}

func testStore(t *testing.T, home, role string) (*Store, context.Context) {
	t.Helper()
	ctx := context.Background()
	st, err := Open(ctx, home, role)
	if err != nil {
		t.Fatal(err)
	}
	return st, ctx
}

func storeNotes(t *testing.T, home, role string) []Note {
	t.Helper()
	st, ctx := testStore(t, home, role)
	defer st.Close()
	notes, err := st.ListNotes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return notes
}

func storeTasks(t *testing.T, home, role string) []Task {
	t.Helper()
	st, ctx := testStore(t, home, role)
	defer st.Close()
	tasks, err := st.ListTasks(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	return tasks
}

func storeReminders(t *testing.T, home, role string) []Reminder {
	t.Helper()
	st, ctx := testStore(t, home, role)
	defer st.Close()
	items, err := st.ListReminders(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func seedNote(t *testing.T, home, role, title, body string) Note {
	t.Helper()
	st, ctx := testStore(t, home, role)
	defer st.Close()
	n, err := st.CreateNote(ctx, title, body)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func seedTask(t *testing.T, home, role, title string, due time.Time) Task {
	t.Helper()
	st, ctx := testStore(t, home, role)
	defer st.Close()
	task, err := st.CreateTask(ctx, title, due)
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func seedReminder(t *testing.T, home, role, title string, due time.Time) Reminder {
	t.Helper()
	st, ctx := testStore(t, home, role)
	defer st.Close()
	rem, err := st.CreateReminder(ctx, title, due)
	if err != nil {
		t.Fatal(err)
	}
	return rem
}

func TestNoteEditorFields(t *testing.T) {
	home := t.TempDir()
	v := newNoteView(home, "sre", record{Kind: kindNote}, nil)
	if got := v.Title(); got != "build note" {
		t.Errorf("fresh note title = %q, want build note", got)
	}

	fields := v.Fields(map[string]any{"title": "idea", "body": "line one\nline two"})
	if got := fieldKeys(fields); !reflect.DeepEqual(got, []string{"title", "body"}) {
		t.Fatalf("note field keys = %v, want title,body", got)
	}
	if got := fieldKinds(fields); !reflect.DeepEqual(got, []forms.FieldKind{forms.FieldText, forms.FieldMultiline}) {
		t.Errorf("note field kinds = %v, want text,multiline", got)
	}
	if !strings.Contains(fields[0].Label, "title") {
		t.Errorf("note title label = %q", fields[0].Label)
	}
	if fields[1].Label != "body" {
		t.Errorf("note body label = %q, want body", fields[1].Label)
	}
	if fields[0].Text != "idea" || fields[1].Text != "line one\nline two" {
		t.Errorf("note seeds = %q/%q, want idea and both body lines", fields[0].Text, fields[1].Text)
	}

	saved := newNoteView(home, "sre", record{Kind: kindNote, ID: 12, Title: "idea", Body: "the body"}, nil)
	if got := saved.Title(); got != "edit note #12" {
		t.Errorf("saved note title = %q, want edit note #12", got)
	}
	if got := saved.Value("title"); got != "idea" {
		t.Errorf("saved note title field = %q, want idea", got)
	}
	if got := forms.Raw(saved.Form().Values(), "body"); got != "the body" {
		t.Errorf("saved note body field = %q, want the body", got)
	}
}

func TestTaskEditorFields(t *testing.T) {
	home := t.TempDir()
	v := newTaskView(home, "sre", record{Kind: kindTask}, nil)
	if got := v.Title(); got != "build task" {
		t.Errorf("fresh task title = %q, want build task", got)
	}

	fields := v.Fields(map[string]any{"title": "ship it", "due": "+2h", "done": true})
	if got := fieldKeys(fields); !reflect.DeepEqual(got, []string{"title", "due", "done"}) {
		t.Fatalf("task field keys = %v, want title,due,done", got)
	}
	if got := fieldKinds(fields); !reflect.DeepEqual(got, []forms.FieldKind{forms.FieldText, forms.FieldText, forms.FieldToggle}) {
		t.Errorf("task field kinds = %v, want text,text,toggle", got)
	}
	if !strings.Contains(fields[1].Label, "due") {
		t.Errorf("task due label = %q", fields[1].Label)
	}
	if fields[2].Label != "done" {
		t.Errorf("task done label = %q, want done", fields[2].Label)
	}
	if fields[0].Text != "ship it" || fields[1].Text != "+2h" {
		t.Errorf("task seeds = %q/%q, want ship it and +2h", fields[0].Text, fields[1].Text)
	}
	if !fields[2].On {
		t.Error("task done did not seed from forms.Bool")
	}
	if off := v.Fields(map[string]any{"done": false}); off[2].On {
		t.Error("task done seeded true from a false value")
	}

	due := recordNow(t).Add(2 * time.Hour)
	saved := newTaskView(home, "sre", record{Kind: kindTask, ID: 3, Title: "ship it", Due: due, Done: true}, nil)
	if got := saved.Title(); got != "edit task #3" {
		t.Errorf("saved task title = %q, want edit task #3", got)
	}
	if got := saved.Value("due"); got != due.UTC().Format(time.RFC3339) {
		t.Errorf("saved task due field = %q, want %q", got, due.UTC().Format(time.RFC3339))
	}
	if !forms.Bool(saved.Form().Values(), "done") {
		t.Error("saved task done field = false, want true")
	}
}

func TestRemindEditorFields(t *testing.T) {
	home := t.TempDir()
	v := newRemindView(home, "sre", record{Kind: kindReminder}, nil)
	if got := v.Title(); got != "build reminder" {
		t.Errorf("fresh reminder title = %q, want build reminder", got)
	}

	fields := v.Fields(map[string]any{"title": "ping", "due": "+2h"})
	if got := fieldKeys(fields); !reflect.DeepEqual(got, []string{"title", "due"}) {
		t.Fatalf("reminder field keys = %v, want title,due only", got)
	}
	if got := fieldKinds(fields); !reflect.DeepEqual(got, []forms.FieldKind{forms.FieldText, forms.FieldText}) {
		t.Errorf("reminder field kinds = %v, want text,text", got)
	}
	if !strings.Contains(fields[1].Label, "due") {
		t.Errorf("reminder due label = %q", fields[1].Label)
	}
	if fields[0].Text != "ping" || fields[1].Text != "+2h" {
		t.Errorf("reminder seeds = %q/%q, want ping and +2h", fields[0].Text, fields[1].Text)
	}
	if got := v.Value("due"); got != remindDueSeed {
		t.Errorf("fresh reminder due seed = %q, want %q", got, remindDueSeed)
	}

	due := recordNow(t).Add(2 * time.Hour)
	saved := newRemindView(home, "sre", record{Kind: kindReminder, ID: 8, Title: "ping", Due: due, Done: true}, nil)
	if got := saved.Title(); got != "edit reminder #8" {
		t.Errorf("saved reminder title = %q, want edit reminder #8", got)
	}
	if got := saved.Value("due"); got != due.UTC().Format(time.RFC3339) {
		t.Errorf("saved reminder due field = %q, want %q", got, due.UTC().Format(time.RFC3339))
	}
	found := false
	for _, c := range saved.Context() {
		if c.Key == "done" && c.Label == "yes" {
			found = true
		}
	}
	if !found {
		t.Errorf("done reminder context = %v, want a done=yes cue", saved.Context())
	}
}

func TestNotePersistCreatesThenUpdatesInPlace(t *testing.T) {
	home := t.TempDir()
	v := newNoteView(home, "r", record{Kind: kindNote}, nil)

	setField(t, v.Form(), "title", " first ")
	setField(t, v.Form(), "body", "body one")
	summary, err := v.Persist()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(summary, "created note #") {
		t.Errorf("first persist summary = %q, want created note #…", summary)
	}
	if v.id == 0 {
		t.Fatal("persist did not write the new id back onto the view")
	}

	setField(t, v.Form(), "title", "second")
	summary, err = v.Persist()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(summary, "updated note #") {
		t.Errorf("second persist summary = %q, want updated note #…", summary)
	}

	notes := storeNotes(t, home, "r")
	if len(notes) != 1 {
		t.Fatalf("notes = %+v, want exactly one row after two persists", notes)
	}
	if notes[0].Title != "second" || notes[0].Body != "body one" {
		t.Errorf("note = %+v, want the second title applied in place", notes[0])
	}
}

func TestTaskPersistRoundTripsDoneAndDue(t *testing.T) {
	home := t.TempDir()
	now := recordNow(t)
	v := newTaskView(home, "r", record{Kind: kindTask}, nil)
	v.now = func() time.Time { return now }

	setField(t, v.Form(), "title", " ship it ")
	setField(t, v.Form(), "due", "+2h")
	setToggle(t, v.Form(), "done", true)
	if _, err := v.Persist(); err != nil {
		t.Fatal(err)
	}

	tasks := storeTasks(t, home, "r")
	if len(tasks) != 1 {
		t.Fatalf("tasks = %+v, want one", tasks)
	}
	if tasks[0].Title != "ship it" {
		t.Errorf("task title = %q, want a trimmed ship it", tasks[0].Title)
	}
	if !tasks[0].Done {
		t.Error("task done = false, want true")
	}
	if want := now.UTC().Add(2 * time.Hour); !tasks[0].Due.Equal(want) {
		t.Errorf("task due = %v, want %v", tasks[0].Due, want)
	}

	setToggle(t, v.Form(), "done", false)
	setField(t, v.Form(), "due", "")
	if _, err := v.Persist(); err != nil {
		t.Fatal(err)
	}
	tasks = storeTasks(t, home, "r")
	if len(tasks) != 1 {
		t.Fatalf("tasks = %+v, want one after the update", tasks)
	}
	if tasks[0].Done {
		t.Error("task done = true, want the update to clear it")
	}
	if !tasks[0].Due.IsZero() {
		t.Errorf("task due = %v, want zero after clearing it", tasks[0].Due)
	}
}

func TestRemindPersistParsesDue(t *testing.T) {
	home := t.TempDir()
	now := recordNow(t)
	v := newRemindView(home, "r", record{Kind: kindReminder}, nil)
	v.now = func() time.Time { return now }

	setField(t, v.Form(), "title", "ping")
	if _, err := v.Persist(); err != nil {
		t.Fatal(err)
	}
	items := storeReminders(t, home, "r")
	if len(items) != 1 {
		t.Fatalf("reminders = %+v, want one", items)
	}
	if want := now.UTC().Add(time.Hour); !items[0].Due.Equal(want) {
		t.Errorf("reminder due = %v, want the +1h seed resolved to %v", items[0].Due, want)
	}

	setField(t, v.Form(), "due", "2026-08-01T09:00:00Z")
	if _, err := v.Persist(); err != nil {
		t.Fatal(err)
	}
	items = storeReminders(t, home, "r")
	if len(items) != 1 {
		t.Fatalf("reminders = %+v, want one after the update", items)
	}
	if want := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC); !items[0].Due.Equal(want) {
		t.Errorf("reminder due = %v, want %v", items[0].Due, want)
	}
}

func TestPersistRejectsBlankTitle(t *testing.T) {
	home := t.TempDir()
	cases := []struct {
		name string
		view interface {
			Form() *forms.Form
			Persist() (string, error)
		}
	}{
		{"note", newNoteView(home, "r", record{Kind: kindNote}, nil)},
		{"task", newTaskView(home, "r", record{Kind: kindTask}, nil)},
		{"reminder", newRemindView(home, "r", record{Kind: kindReminder}, nil)},
	}
	for _, c := range cases {
		setField(t, c.view.Form(), "title", "   ")
		summary, err := c.view.Persist()
		if err == nil {
			t.Errorf("%s: blank title persisted with summary %q", c.name, summary)
			continue
		}
		if !strings.Contains(err.Error(), "needs a title") {
			t.Errorf("%s: error = %q, want it to mention a missing title", c.name, err)
		}
	}
	if notes := storeNotes(t, home, "r"); len(notes) != 0 {
		t.Errorf("notes = %+v, want none written", notes)
	}
	if tasks := storeTasks(t, home, "r"); len(tasks) != 0 {
		t.Errorf("tasks = %+v, want none written", tasks)
	}
	if items := storeReminders(t, home, "r"); len(items) != 0 {
		t.Errorf("reminders = %+v, want none written", items)
	}
}

func TestRemindPersistRejectsBlankDue(t *testing.T) {
	home := t.TempDir()
	v := newRemindView(home, "r", record{Kind: kindReminder}, nil)
	setField(t, v.Form(), "title", "ping")
	setField(t, v.Form(), "due", "")

	summary, err := v.Persist()
	if err == nil {
		t.Fatalf("a reminder with no due persisted with summary %q", summary)
	}
	if !strings.Contains(err.Error(), "never fires") {
		t.Errorf("error = %q, want it to mention that it never fires", err)
	}
	if items := storeReminders(t, home, "r"); len(items) != 0 {
		t.Errorf("reminders = %+v, want none written", items)
	}
}

func TestEditorFooterOmitsWrite(t *testing.T) {
	home := t.TempDir()
	fresh := newNoteView(home, "r", record{Kind: kindNote}, nil)
	hints := fresh.Hints()
	for _, want := range []string{"ctrl+r", "ctrl+t", "ctrl+y", "ctrl+s", "ctrl+g"} {
		if !hasHint(hints, want) {
			t.Errorf("note builder hints missing %s: %v", want, hints)
		}
	}
	if hasHint(hints, "ctrl+w") {
		t.Errorf("note builder offers ctrl+w but a note has no file output: %v", hints)
	}
	if hasHint(hints, "ctrl+x") {
		t.Errorf("note builder offers delete before it is saved: %v", hints)
	}
	if hasHint(hints, "tab") {
		t.Errorf("note builder offers tab before any run: %v", hints)
	}

	n := seedNote(t, home, "r", "idea", "the body")
	saved := newNoteView(home, "r", noteRecord(n), nil)
	if !hasHint(saved.Hints(), "ctrl+x") {
		t.Errorf("saved note hints missing ctrl+x: %v", saved.Hints())
	}

	app := deck.New(saved)
	app = step(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	app = settle(app, cmdOf(update(app, tea.KeyMsg{Type: tea.KeyCtrlR})))
	if saved.Running() {
		t.Fatalf("run never landed (status %q)", saved.Status())
	}
	if !hasHint(saved.Hints(), "tab") {
		t.Errorf("hints missing tab after a successful run: %v", saved.Hints())
	}
	step(app, tea.KeyMsg{Type: tea.KeyTab})
	if saved.OnResults() {
		t.Fatal("tab did not return focus to the form")
	}
	if !hasHint(saved.Hints(), "tab") {
		t.Errorf("form hints missing tab while results are loaded: %v", saved.Hints())
	}
}

func TestEditorDeleteHiddenForNewRecord(t *testing.T) {
	v := newNoteView(t.TempDir(), "r", record{Kind: kindNote}, nil)
	if got := v.SavedName(); got != "" {
		t.Fatalf("SavedName = %q, want empty for a builder", got)
	}

	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	step(app, tea.KeyMsg{Type: tea.KeyCtrlX})

	if v.Confirming() {
		t.Error("ctrl+x opened a delete dialog for an unsaved note")
	}
	if !strings.Contains(v.Status(), "has not been saved yet") {
		t.Errorf("status = %q, want it to say the note has not been saved yet", v.Status())
	}
}

func TestEditorDeleteRemovesRecord(t *testing.T) {
	home := t.TempDir()
	n := seedNote(t, home, "r", "drop me", "")
	v := newNoteView(home, "r", noteRecord(n), nil)
	if want := "note #" + strconv.FormatInt(n.ID, 10); v.SavedName() != want {
		t.Fatalf("SavedName = %q, want %q", v.SavedName(), want)
	}

	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	app = step(app, tea.KeyMsg{Type: tea.KeyCtrlX})
	if !v.Confirming() {
		t.Fatalf("ctrl+x did not open the confirm dialog (status %q)", v.Status())
	}
	if got := app.View(); !strings.Contains(got, "delete note #") {
		t.Fatalf("confirm dialog missing its title: %q", got)
	}

	app = step(app, tea.KeyMsg{Type: tea.KeyLeft})
	settle(app, cmdOf(update(app, tea.KeyMsg{Type: tea.KeyEnter})))

	if notes := storeNotes(t, home, "r"); len(notes) != 0 {
		t.Fatalf("note survived the delete: %+v", notes)
	}
}

func TestEditorCopyKeyIsWired(t *testing.T) {
	home := t.TempDir()
	rec := record{Kind: kindNote, ID: 4, Title: "idea", Body: "the body"}
	v := newNoteView(home, "r", rec, nil)
	copies := 0
	payload := ""
	v.copy = func(text string) error {
		copies++
		payload = text
		return nil
	}

	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	app = settle(app, cmdOf(update(app, tea.KeyMsg{Type: tea.KeyCtrlG})))

	if copies != 1 {
		t.Fatalf("ctrl+g copied %d times, want 1 (status %q)", copies, v.Status())
	}
	if want := rec.text(); payload != want {
		t.Errorf("copied %q, want the record text %q", payload, want)
	}
	if app.Top().Title() != "copy" {
		t.Fatalf("top view after ctrl+g = %q, want a pushed copy message", app.Top().Title())
	}
	if v.Status() != "" {
		t.Errorf("status = %q, want it cleared after a successful copy", v.Status())
	}
}

func TestEditorValidateSurfacesCheckLines(t *testing.T) {
	home := t.TempDir()
	now := recordNow(t)
	v := newTaskView(home, "r", record{Kind: kindTask, Title: "ship it"}, nil)
	v.now = func() time.Time { return now }

	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	app = step(app, tea.KeyMsg{Type: tea.KeyCtrlT})

	notice := strings.Join(v.Notice(), "\n")
	if notice == "" {
		t.Fatalf("ctrl+t left the validation panel empty (status %q)", v.Status())
	}
	for _, want := range []string{"looks good", "no due set"} {
		if !strings.Contains(notice, want) {
			t.Errorf("validation lines missing %q: %q", want, notice)
		}
	}
	if v.Status() != "" {
		t.Errorf("status = %q, want it cleared on a valid check", v.Status())
	}

	setField(t, v.Form(), "title", "  ")
	step(app, tea.KeyMsg{Type: tea.KeyCtrlT})
	if !strings.Contains(v.Status(), "needs a title") {
		t.Errorf("status = %q, want the missing-title error", v.Status())
	}
	if len(v.Notice()) != 0 {
		t.Errorf("validation panel = %v, want it cleared when the record cannot be built", v.Notice())
	}
}
