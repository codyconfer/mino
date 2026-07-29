package ntr

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/plugin"
	pub "github.com/codyconfer/munin/plugin"
)

func TestParseDueRelative(t *testing.T) {
	before := time.Now()
	got, err := parseDue("+2h")
	if err != nil {
		t.Fatal(err)
	}
	after := time.Now()
	assertBetween(t, got, before.Add(2*time.Hour), after.Add(2*time.Hour))
	if got.Location() != time.UTC {
		t.Fatalf("location = %v, want UTC", got.Location())
	}
}

func TestParseDueEmptyDefaultsToAnHourOut(t *testing.T) {
	for _, in := range []string{"", "   "} {
		before := time.Now()
		got, err := parseDue(in)
		if err != nil {
			t.Fatalf("parseDue(%q): %v", in, err)
		}
		after := time.Now()
		assertBetween(t, got, before.Add(time.Hour), after.Add(time.Hour))
		if got.Location() != time.UTC {
			t.Fatalf("parseDue(%q) location = %v, want UTC", in, got.Location())
		}
	}
}

func TestParseDueAbsolute(t *testing.T) {
	rfc := "2031-04-05T06:07:08+02:00"
	wantRFC, err := time.Parse(time.RFC3339, rfc)
	if err != nil {
		t.Fatal(err)
	}
	wantDate, err := time.ParseInLocation("2006-01-02", "2031-04-05", time.Local)
	if err != nil {
		t.Fatal(err)
	}
	wantMinute, err := time.ParseInLocation("2006-01-02 15:04", "2031-04-05 06:07", time.Local)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		in   string
		want time.Time
	}{
		{rfc, wantRFC},
		{"2031-04-05", wantDate},
		{"2031-04-05 06:07", wantMinute},
	}
	for _, c := range cases {
		got, err := parseDue(c.in)
		if err != nil {
			t.Fatalf("parseDue(%q): %v", c.in, err)
		}
		if !got.Equal(c.want) {
			t.Errorf("parseDue(%q) = %v, want %v", c.in, got, c.want)
		}
		if got.Location() != time.UTC {
			t.Errorf("parseDue(%q) location = %v, want UTC", c.in, got.Location())
		}
	}
}

func TestParseDueBad(t *testing.T) {
	for _, in := range []string{"not-a-date", "+nope", "2031-13-45"} {
		got, err := parseDue(in)
		if err == nil {
			t.Fatalf("parseDue(%q) = %v, want error", in, got)
		}
		if !strings.HasPrefix(err.Error(), "due:") {
			t.Errorf("parseDue(%q) error = %q, want a due: prefix", in, err)
		}
	}
}

func assertBetween(t *testing.T, got, lo, hi time.Time) {
	t.Helper()
	if got.Before(lo.Add(-time.Second)) || got.After(hi.Add(time.Second)) {
		t.Fatalf("got %v, want within [%v, %v]", got, lo, hi)
	}
}

func TestNTRViewsRegistered(t *testing.T) {
	for _, id := range []string{"ntr.home", "ntr.notes", "ntr.tasks", "ntr.reminders"} {
		if _, ok := vkdeck.LookupView(id); !ok {
			t.Fatalf("missing view %s (have %v)", id, vkdeck.ViewIDs())
		}
	}
	if !strings.Contains(strings.Join(vkdeck.ViewIDs(), ","), "ntr.") {
		t.Fatal(vkdeck.ViewIDs())
	}
}

func TestRemindersViewServiceOnly(t *testing.T) {
	d, ok := plugin.ByKind(plugin.KindView, "ntr.reminders")
	if !ok {
		t.Fatal("missing ntr.reminders descriptor")
	}
	if !d.ServiceOnly {
		t.Fatalf("ntr.reminders ServiceOnly = false, want true: %+v", d)
	}

	pub.SetServiceAttachedFunc(func() bool { return false })
	t.Cleanup(func() { pub.SetServiceAttachedFunc(plugin.ServiceAttached) })
	if RemindersUIVisible() {
		t.Fatal("reminders should be hidden when service detached")
	}

	v := NewHomeView(t.TempDir(), "default")
	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	got := app.View()
	if strings.Contains(got, "Reminders") {
		t.Fatalf("home menu showed Reminders while detached: %q", got)
	}
	for _, want := range []string{"Notes", "Tasks"} {
		if !strings.Contains(got, want) {
			t.Fatalf("home menu missing %q while detached: %q", want, got)
		}
	}

	pub.SetServiceAttachedFunc(func() bool { return true })
	if !RemindersUIVisible() {
		t.Fatal("reminders should be visible when service attached")
	}
	v = NewHomeView(t.TempDir(), "default")
	app = deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	got = app.View()
	if !strings.Contains(got, "Reminders") {
		t.Fatalf("home menu missing Reminders while attached: %q", got)
	}
}

func step(a *vkdeck.Model, msg tea.Msg) *vkdeck.Model {
	m, _ := a.Update(msg)
	return m.(*vkdeck.Model)
}

func formValueKeys(v *vkdeck.FormView) []string {
	keys := make([]string, 0, len(v.Values()))
	for k := range v.Values() {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestNoteFormBuildsSeededFields(t *testing.T) {
	v := newNoteForm(t.TempDir(), "sre", 0, "", "", nil)
	if got := formValueKeys(v); !reflect.DeepEqual(got, []string{"body", "title"}) {
		t.Fatalf("field keys = %v, want body,title", got)
	}
	if v.Title() != "new note" {
		t.Errorf("title = %q, want new note", v.Title())
	}
	if !reflect.DeepEqual(v.Context(), [][2]string{{"role", "sre"}}) {
		t.Errorf("context = %v", v.Context())
	}
	if !reflect.DeepEqual(v.Hints(), [][2]string{{"↑/↓", "field"}, {"ctrl+s", "save"}}) {
		t.Errorf("hints = %v", v.Hints())
	}

	edit := newNoteForm(t.TempDir(), "sre", 7, "seed", "text", nil)
	if edit.Title() != "edit note" {
		t.Errorf("title = %q, want edit note", edit.Title())
	}
	vals := edit.Values()
	if vals["title"] != "seed" || vals["body"] != "text" {
		t.Fatalf("seeded values = %v, want title=seed body=text", vals)
	}
}

func TestTaskFormBuildsFields(t *testing.T) {
	v := newTaskForm(t.TempDir(), "sre", nil)
	if got := formValueKeys(v); !reflect.DeepEqual(got, []string{"title"}) {
		t.Fatalf("field keys = %v, want title", got)
	}
	if v.Title() != "new task" {
		t.Errorf("title = %q, want new task", v.Title())
	}
	if !reflect.DeepEqual(v.Hints(), [][2]string{{"ctrl+s", "save"}}) {
		t.Errorf("hints = %v", v.Hints())
	}
}

func TestRemindFormBuildsFieldsWithDefaultDue(t *testing.T) {
	v := newRemindForm(t.TempDir(), "sre", nil)
	if got := formValueKeys(v); !reflect.DeepEqual(got, []string{"due", "title"}) {
		t.Fatalf("field keys = %v, want due,title", got)
	}
	if v.Title() != "new reminder" {
		t.Errorf("title = %q, want new reminder", v.Title())
	}
	if got := v.Values()["due"]; got != "+1h" {
		t.Fatalf("due default = %v, want +1h", got)
	}
}

func runSaver(t *testing.T, save formSaver, vals map[string]any) savedMsg {
	t.Helper()
	cmd, problem := save(vals)
	if problem != "" {
		t.Fatalf("unexpected validation error %q", problem)
	}
	if cmd == nil {
		t.Fatal("saver returned no command")
	}
	msg, ok := cmd().(savedMsg)
	if !ok {
		t.Fatalf("save command produced %T, want savedMsg", msg)
	}
	if msg.err != "" {
		t.Fatalf("save failed: %s", msg.err)
	}
	return msg
}

func TestSaveNoteCreatesThenUpdates(t *testing.T) {
	home := t.TempDir()
	reloaded := false
	reload := func() tea.Cmd { reloaded = true; return nil }

	msg := runSaver(t, saveNote(home, "r", 0, reload), map[string]any{"title": " idea ", "body": "text"})
	if msg.reload == nil {
		t.Fatal("savedMsg should carry the reload callback")
	}
	msg.reload()
	if !reloaded {
		t.Error("reload callback was not the one passed in")
	}

	ctx := context.Background()
	st, err := Open(ctx, home, "r")
	if err != nil {
		t.Fatal(err)
	}
	notes, err := st.ListNotes(ctx)
	st.Close()
	if err != nil || len(notes) != 1 {
		t.Fatalf("notes = %v err=%v", notes, err)
	}
	if notes[0].Title != "idea" || notes[0].Body != "text" {
		t.Fatalf("note = %+v, want a trimmed title and its body", notes[0])
	}

	runSaver(t, saveNote(home, "r", notes[0].ID, nil), map[string]any{"title": "idea2", "body": "text2"})
	st, err = Open(ctx, home, "r")
	if err != nil {
		t.Fatal(err)
	}
	notes, err = st.ListNotes(ctx)
	st.Close()
	if err != nil || len(notes) != 1 {
		t.Fatalf("notes = %v err=%v", notes, err)
	}
	if notes[0].Title != "idea2" || notes[0].Body != "text2" {
		t.Fatalf("note = %+v, want the update applied in place", notes[0])
	}
}

func TestSaveTaskCreates(t *testing.T) {
	home := t.TempDir()
	runSaver(t, saveTask(home, "r", nil), map[string]any{"title": " ship "})

	ctx := context.Background()
	st, err := Open(ctx, home, "r")
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := st.ListTasks(ctx, true)
	st.Close()
	if err != nil || len(tasks) != 1 || tasks[0].Title != "ship" {
		t.Fatalf("tasks = %v err=%v", tasks, err)
	}
}

func TestSaveRemindCreatesWithParsedDue(t *testing.T) {
	home := t.TempDir()
	runSaver(t, saveRemind(home, "r", nil), map[string]any{"title": "ping", "due": "+2h"})

	ctx := context.Background()
	st, err := Open(ctx, home, "r")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	items, err := st.ListReminders(ctx, true)
	if err != nil || len(items) != 1 || items[0].Title != "ping" {
		t.Fatalf("reminders = %v err=%v", items, err)
	}

	now := time.Now().UTC()
	early, err := st.DueReminders(ctx, now.Add(time.Hour))
	if err != nil || len(early) != 0 {
		t.Fatalf("a +2h reminder is not due in an hour: %v err=%v", early, err)
	}
	late, err := st.DueReminders(ctx, now.Add(3*time.Hour))
	if err != nil || len(late) != 1 {
		t.Fatalf("a +2h reminder should be due in three hours: %v err=%v", late, err)
	}
}

func TestSaversRejectABlankTitle(t *testing.T) {
	savers := map[string]formSaver{
		"note":     saveNote(t.TempDir(), "r", 0, nil),
		"task":     saveTask(t.TempDir(), "r", nil),
		"reminder": saveRemind(t.TempDir(), "r", nil),
	}
	for name, save := range savers {
		cmd, problem := save(map[string]any{"title": "   ", "due": "+1h"})
		if cmd != nil {
			t.Errorf("%s: blank title returned a save command", name)
		}
		if problem != "title required" {
			t.Errorf("%s: problem = %q, want title required", name, problem)
		}
	}
}

func TestSaveRemindRejectsABadDue(t *testing.T) {
	cmd, problem := saveRemind(t.TempDir(), "r", nil)(map[string]any{"title": "ping", "due": "not-a-date"})
	if cmd != nil {
		t.Error("a bad due should not reach the store")
	}
	if !strings.HasPrefix(problem, "due:") {
		t.Fatalf("problem = %q, want a due: prefix", problem)
	}
}

func hostWithForm(t *testing.T, v vkdeck.View) *vkdeck.Model {
	t.Helper()
	h := deck.New(NewHomeView(t.TempDir(), "default"))
	h.Push(v)
	if h.Top() != v {
		t.Fatal("form is not on top of the stack")
	}
	return h
}

func TestFormSavedMsgSuccessPopsAndReloads(t *testing.T) {
	v := newTaskForm(t.TempDir(), "r", nil)
	h := hostWithForm(t, v)

	reloaded := false
	cmd := v.Update(h, savedMsg{reload: func() tea.Cmd { reloaded = true; return nil }})
	if cmd == nil {
		t.Error("a successful save should return the pop/reload batch")
	}
	if !reloaded {
		t.Error("the list reload was not triggered")
	}
	if h.Top().Title() != "notes" {
		t.Fatalf("top view = %q, want the pushed-from view", h.Top().Title())
	}
}

func TestFormSavedMsgFailureShowsTheError(t *testing.T) {
	v := newNoteForm(t.TempDir(), "r", 0, "", "", nil)
	h := hostWithForm(t, v)

	if cmd := v.Update(h, savedMsg{err: "disk on fire"}); cmd != nil {
		t.Errorf("a failed save should not return a command, got %v", cmd)
	}
	if h.Top() != vkdeck.View(v) {
		t.Fatal("a failed save must keep the form open")
	}
	if !strings.Contains(v.Body(80, 24), "disk on fire") {
		t.Fatalf("body is missing the error:\n%s", v.Body(80, 24))
	}
}

func TestFormSaveKeySurfacesValidationErrors(t *testing.T) {
	v := newTaskForm(t.TempDir(), "r", nil)
	h := hostWithForm(t, v)

	if cmd := v.Update(h, tea.KeyMsg{Type: tea.KeyCtrlS}); cmd != nil {
		t.Errorf("an invalid save should not return a command, got %v", cmd)
	}
	if !strings.Contains(v.Body(80, 24), "title required") {
		t.Fatalf("body is missing the validation error:\n%s", v.Body(80, 24))
	}

	for _, r := range "ship" {
		v.Update(h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if cmd := v.Update(h, tea.KeyMsg{Type: tea.KeyCtrlS}); cmd == nil {
		t.Fatal("a valid save should return the store command")
	}
	if strings.Contains(v.Body(80, 24), "title required") {
		t.Errorf("the stale validation error survived a valid save:\n%s", v.Body(80, 24))
	}
}

func TestFormCancelPops(t *testing.T) {
	v := newRemindForm(t.TempDir(), "r", nil)
	h := hostWithForm(t, v)
	v.Update(h, tea.KeyMsg{Type: tea.KeyEsc})
	if h.Top().Title() != "notes" {
		t.Fatalf("top view = %q, want the form popped", h.Top().Title())
	}
}

func TestRenderRowsSpacesOnlyTheCursorRow(t *testing.T) {
	rows := []string{"1  first", "2  second", "3  third"}
	lines := strings.Split(renderRows(80, 1, "NOTES", rows, "(none)"), "\n")

	at := func(want string) int {
		for i, ln := range lines {
			if strings.Contains(ln, want) {
				return i
			}
		}
		return -1
	}
	first, second, third := at("first"), at("second"), at("third")
	if first < 0 || second < 0 || third < 0 {
		t.Fatalf("rows missing:\n%s", strings.Join(lines, "\n"))
	}
	if second-first != 2 || third-second != 2 {
		t.Errorf("the cursor row should be blank-separated on both sides:\n%s", strings.Join(lines, "\n"))
	}

	tight := strings.Split(renderRows(80, 0, "NOTES", rows, "(none)"), "\n")
	at = func(want string) int {
		for i, ln := range tight {
			if strings.Contains(ln, want) {
				return i
			}
		}
		return -1
	}
	if at("third")-at("second") != 1 {
		t.Errorf("rows away from the cursor should stay tight:\n%s", strings.Join(tight, "\n"))
	}
}
