package ntr

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"time"

	"github.com/codyconfer/sisyphus/kv"

	"github.com/codyconfer/munin/internal/render"
)

// CLI helpers keep cobra shells thin in cmd/ (M7).

func openCLI(ctx context.Context, home, role string) (*Store, error) {
	if role == "" {
		role = "default"
	}
	return Open(ctx, home, role)
}

// CLINotesList writes open notes as id\ttitle lines.
func CLINotesList(ctx context.Context, w io.Writer, home, role string) error {
	st, err := openCLI(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	notes, err := st.ListNotes(ctx)
	if err != nil {
		return err
	}
	for _, n := range notes {
		fmt.Fprintf(w, "%d\t%s\n", n.ID, n.Title)
	}
	return nil
}

// CLINotesAdd creates a note.
func CLINotesAdd(ctx context.Context, w io.Writer, home, role, title, body string) error {
	st, err := openCLI(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	n, err := st.CreateNote(ctx, title, body)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, render.Success(fmt.Sprintf("note %d created", n.ID)))
	return nil
}

// CLINotesRM deletes a note by id string.
func CLINotesRM(ctx context.Context, w io.Writer, home, role, idStr string) error {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return err
	}
	st, err := openCLI(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.DeleteNote(ctx, id); err != nil {
		return err
	}
	fmt.Fprintln(w, render.Success("deleted"))
	return nil
}

// CLITasksList writes open tasks.
func CLITasksList(ctx context.Context, w io.Writer, home, role string) error {
	st, err := openCLI(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	tasks, err := st.ListTasks(ctx, false)
	if err != nil {
		return err
	}
	for _, t := range tasks {
		fmt.Fprintf(w, "%d\t%s\n", t.ID, t.Title)
	}
	return nil
}

// CLITasksAdd creates a task.
func CLITasksAdd(ctx context.Context, w io.Writer, home, role, title string) error {
	st, err := openCLI(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	t, err := st.CreateTask(ctx, title, time.Time{})
	if err != nil {
		return err
	}
	fmt.Fprintln(w, render.Success(fmt.Sprintf("task %d created", t.ID)))
	return nil
}

// CLITasksDone marks a task done.
func CLITasksDone(ctx context.Context, w io.Writer, home, role, idStr string) error {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return err
	}
	st, err := openCLI(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.SetTaskDone(ctx, id, true); err != nil {
		return err
	}
	fmt.Fprintln(w, render.Success("done"))
	return nil
}

// CLIRemindList writes open reminders.
func CLIRemindList(ctx context.Context, w io.Writer, home, role string) error {
	st, err := openCLI(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	rems, err := st.ListReminders(ctx, false)
	if err != nil {
		return err
	}
	for _, r := range rems {
		fmt.Fprintf(w, "%d\t%s\t%s\n", r.ID, r.Due.Format(time.RFC3339), r.Title)
	}
	return nil
}

// CLIRemindAdd creates a reminder due after duration.
func CLIRemindAdd(ctx context.Context, w io.Writer, home, role, title, dur string) error {
	d, err := time.ParseDuration(dur)
	if err != nil {
		return err
	}
	st, err := openCLI(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	r, err := st.CreateReminder(ctx, title, time.Now().Add(d))
	if err != nil {
		return err
	}
	fmt.Fprintln(w, render.Success(fmt.Sprintf("reminder %d at %s", r.ID, r.Due.Format(time.RFC3339))))
	return nil
}

// CLICatchUp fires due reminders (Fetch → print → Ack) using serve.duckdb watermark.
func CLICatchUp(ctx context.Context, w io.Writer, home, role string) error {
	if role == "" {
		role = "default"
	}
	store, err := kv.Open(ctx, filepath.Join(home, "serve.duckdb"))
	if err != nil {
		return err
	}
	defer store.Close()
	job := ReminderJob{Home: home, Role: role, KV: store}
	secs, err := job.Fetch(ctx)
	if err != nil {
		return err
	}
	n := 0
	if len(secs) > 0 {
		n = len(secs[0].Items)
	}
	fmt.Fprintf(w, "fired %d reminder(s)\n", n)
	for _, sec := range secs {
		for _, it := range sec.Items {
			fmt.Fprintf(w, "- %s\n", it.Title)
		}
	}
	return job.Ack(ctx, secs)
}
