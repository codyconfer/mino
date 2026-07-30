package ntr

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/munin/internal/plugin"
)

func TestStoreCRUDAndReminders(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	st, err := Open(ctx, home, "work")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	n, err := st.CreateNote(ctx, "idea", "body")
	if err != nil || n.ID == 0 {
		t.Fatalf("note = %+v err=%v", n, err)
	}
	notes, err := st.ListNotes(ctx)
	if err != nil || len(notes) != 1 {
		t.Fatalf("notes = %v err=%v", notes, err)
	}

	task, err := st.CreateTask(ctx, "ship", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetTaskDone(ctx, task.ID, true); err != nil {
		t.Fatal(err)
	}
	open, err := st.ListTasks(ctx, false)
	if err != nil || len(open) != 0 {
		t.Fatalf("open tasks = %v", open)
	}

	due := time.Now().UTC().Add(-time.Minute)
	rem, err := st.CreateReminder(ctx, "ping", due)
	if err != nil {
		t.Fatal(err)
	}
	dueList, err := st.DueReminders(ctx, time.Now().UTC())
	if err != nil || len(dueList) != 1 || dueList[0].ID != rem.ID {
		t.Fatalf("due = %v err=%v", dueList, err)
	}
}

func TestSignalFetch(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	st, err := Open(ctx, home, "r")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = st.CreateNote(ctx, "n", "")
	st.Close()

	secs, err := (Signal{Home: home, Role: "r"}).Fetch(ctx)
	if err != nil || len(secs) != 1 || len(secs[0].Items) != 1 {
		t.Fatalf("Fetch = %v err=%v", secs, err)
	}
}

func TestPluginRegistered(t *testing.T) {
	d, ok := plugin.Lookup(PluginID)
	if !ok || d.Signal != SignalName {
		t.Fatalf("lookup = %+v ok=%v", d, ok)
	}
	if !plugin.HasCapability(SignalName, plugin.CapScheduled) {
		t.Fatal("expected CapScheduled")
	}
}

func TestReminderJobCatchUp(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	st, err := Open(ctx, home, "r")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = st.CreateReminder(ctx, "late", time.Now().UTC().Add(-time.Hour))
	st.Close()

	job := ReminderJob{Home: home, Role: "r", Now: time.Now}
	_, ready, err := job.Next(ctx, time.Now())
	if err != nil || !ready {
		t.Fatalf("Next ready=%v err=%v", ready, err)
	}
	secs, err := job.Fetch(ctx)
	if err != nil || len(secs[0].Items) != 1 {
		t.Fatalf("Fetch = %v err=%v", secs, err)
	}

	st, err = Open(ctx, home, "r")
	if err != nil {
		t.Fatal(err)
	}
	due, err := st.DueReminders(ctx, time.Now().UTC())
	st.Close()
	if err != nil || len(due) != 1 {
		t.Fatalf("after Fetch still due = %v err=%v", due, err)
	}

	if err := job.Ack(ctx, secs); err != nil {
		t.Fatal(err)
	}
	st, err = Open(ctx, home, "r")
	if err != nil {
		t.Fatal(err)
	}
	due, err = st.DueReminders(ctx, time.Now().UTC())
	st.Close()
	if err != nil || len(due) != 0 {
		t.Fatalf("after Ack due = %v err=%v", due, err)
	}
}

func TestDueTodayCountLocalDay(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	st, err := Open(ctx, home, "r")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	loc := time.FixedZone("test", -7*3600)
	now := time.Date(2026, 7, 24, 1, 0, 0, 0, loc)
	_, err = st.CreateReminder(ctx, "local-morning", time.Date(2026, 7, 24, 9, 0, 0, 0, loc))
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateReminder(ctx, "prev-local-day", time.Date(2026, 7, 23, 20, 0, 0, 0, loc))
	if err != nil {
		t.Fatal(err)
	}
	n, err := st.DueTodayCount(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("DueTodayCount = %d, want 1 (local day)", n)
	}
}

func TestUpdateTaskClearsDueToNull(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, t.TempDir(), "r")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	due := time.Date(2026, 7, 29, 2, 10, 54, 0, time.UTC)
	task, err := st.CreateTask(ctx, "ship", due)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateTask(ctx, task.ID, "shipped", true, time.Time{}); err != nil {
		t.Fatal(err)
	}
	ts, err := st.ListTasks(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 1 {
		t.Fatalf("tasks = %d, want 1", len(ts))
	}
	if !ts[0].Due.IsZero() {
		t.Errorf("task Due = %v, want zero", ts[0].Due)
	}
	if ts[0].Title != "shipped" {
		t.Errorf("task Title = %q, want %q", ts[0].Title, "shipped")
	}
	if !ts[0].Done {
		t.Error("task Done = false, want true")
	}
}

func TestUpdateTaskRoleScoped(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	a, err := Open(ctx, home, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	task, err := a.CreateTask(ctx, "alpha task", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	a.Close()

	b, err := Open(ctx, home, "bravo")
	if err != nil {
		t.Fatal(err)
	}
	err = b.UpdateTask(ctx, task.ID, "hijacked", true, time.Time{})
	b.Close()
	if err == nil {
		t.Fatal("UpdateTask from another role succeeded, want error")
	}
	if !strings.Contains(err.Error(), "no longer exists") {
		t.Fatalf("err = %v, want it to mention no longer exists", err)
	}

	a, err = Open(ctx, home, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	ts, err := a.ListTasks(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 1 {
		t.Fatalf("tasks = %d, want 1", len(ts))
	}
	if ts[0].Title != "alpha task" || ts[0].Done {
		t.Fatalf("task = %+v, want unchanged", ts[0])
	}
}

func TestUpdateReminderRoundTrip(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, t.TempDir(), "r")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	rem, err := st.CreateReminder(ctx, "ping", time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	due := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	if err := st.UpdateReminder(ctx, rem.ID, "pong", due); err != nil {
		t.Fatal(err)
	}
	rs, err := st.ListReminders(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 {
		t.Fatalf("reminders = %d, want 1", len(rs))
	}
	if rs[0].Title != "pong" {
		t.Errorf("reminder Title = %q, want %q", rs[0].Title, "pong")
	}
	if !rs[0].Due.Equal(due) {
		t.Errorf("reminder Due = %v, want %v", rs[0].Due, due)
	}
	dueList, err := st.DueReminders(ctx, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(dueList) != 1 || dueList[0].ID != rem.ID {
		t.Fatalf("DueReminders = %v, want the updated reminder", dueList)
	}
}

func TestDeleteReminderRoleScoped(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	a, err := Open(ctx, home, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	rem, err := a.CreateReminder(ctx, "keep me", time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	a.Close()

	b, err := Open(ctx, home, "bravo")
	if err != nil {
		t.Fatal(err)
	}
	err = b.DeleteReminder(ctx, rem.ID)
	b.Close()
	if err != nil {
		t.Fatal(err)
	}

	a, err = Open(ctx, home, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	rs, err := a.ListReminders(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 {
		t.Fatalf("reminders after wrong-role delete = %d, want 1", len(rs))
	}
	if err := a.DeleteReminder(ctx, rem.ID); err != nil {
		t.Fatal(err)
	}
	rs, err = a.ListReminders(ctx, true)
	a.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 0 {
		t.Fatalf("reminders after delete = %d, want 0", len(rs))
	}
}

func TestUpdateRejectsMissingRecord(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, t.TempDir(), "r")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	err = st.UpdateTask(ctx, 4242, "ghost", false, time.Time{})
	if err == nil || !strings.Contains(err.Error(), "no longer exists") {
		t.Fatalf("UpdateTask err = %v, want no longer exists", err)
	}
	err = st.UpdateReminder(ctx, 4243, "ghost", time.Now().UTC())
	if err == nil || !strings.Contains(err.Error(), "no longer exists") {
		t.Fatalf("UpdateReminder err = %v, want no longer exists", err)
	}
}

func TestDBPath(t *testing.T) {
	home := t.TempDir()
	st, err := Open(context.Background(), home, "r")
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	if _, err := filepath.Rel(home, filepath.Join(home, "ntr.duckdb")); err != nil {
		t.Fatal(err)
	}
}
