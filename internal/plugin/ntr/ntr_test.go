package ntr

import (
	"context"
	"path/filepath"
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
