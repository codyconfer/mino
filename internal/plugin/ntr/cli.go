package ntr

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/codyconfer/sisyphus/kv"

	"github.com/codyconfer/viewkit/ui"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/plugin"
	"github.com/codyconfer/mino/internal/render"
)

func openCLI(ctx context.Context, home, role string) (*Store, error) {
	if role == "" {
		role = "default"
	}
	return Open(ctx, home, role)
}

func CLINotesList(ctx context.Context, w io.Writer, scope *ui.Scope, home, role string) error {
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

func CLINotesAdd(ctx context.Context, w io.Writer, scope *ui.Scope, home, role, title, body string) error {
	st, err := openCLI(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	n, err := st.CreateNote(ctx, title, body)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, render.Success(scope, fmt.Sprintf("note %d created", n.ID)))
	return nil
}

func CLINotesUpdate(ctx context.Context, w io.Writer, scope *ui.Scope, home, role, idStr, title, body string) error {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return err
	}
	st, err := openCLI(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.UpdateNote(ctx, id, title, body); err != nil {
		return err
	}
	fmt.Fprintln(w, render.Success(scope, fmt.Sprintf("note %d updated", id)))
	return nil
}

func CLINotesRM(ctx context.Context, w io.Writer, scope *ui.Scope, home, role, idStr string) error {
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
	fmt.Fprintln(w, render.Success(scope, "deleted"))
	return nil
}

func CLITasksList(ctx context.Context, w io.Writer, scope *ui.Scope, home, role string) error {
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

func CLITasksAdd(ctx context.Context, w io.Writer, scope *ui.Scope, home, role, title string) error {
	st, err := openCLI(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	t, err := st.CreateTask(ctx, title, time.Time{})
	if err != nil {
		return err
	}
	fmt.Fprintln(w, render.Success(scope, fmt.Sprintf("task %d created", t.ID)))
	return nil
}

func CLITasksDone(ctx context.Context, w io.Writer, scope *ui.Scope, home, role, idStr string) error {
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
	fmt.Fprintln(w, render.Success(scope, "done"))
	return nil
}

func CLITasksUndo(ctx context.Context, w io.Writer, scope *ui.Scope, home, role, idStr string) error {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return err
	}
	st, err := openCLI(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.SetTaskDone(ctx, id, false); err != nil {
		return err
	}
	fmt.Fprintln(w, render.Success(scope, "reopened"))
	return nil
}

func CLITasksRM(ctx context.Context, w io.Writer, scope *ui.Scope, home, role, idStr string) error {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return err
	}
	st, err := openCLI(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.DeleteTask(ctx, id); err != nil {
		return err
	}
	fmt.Fprintln(w, render.Success(scope, "deleted"))
	return nil
}

func CLIRemindList(ctx context.Context, w io.Writer, scope *ui.Scope, home, role string) error {
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

func CLIRemindAdd(ctx context.Context, w io.Writer, scope *ui.Scope, home, role, title, dur string) error {
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
	fmt.Fprintln(w, render.Success(scope, fmt.Sprintf("reminder %d at %s", r.ID, r.Due.Format(time.RFC3339))))
	return nil
}

func CLIRemindDone(ctx context.Context, w io.Writer, scope *ui.Scope, home, role, idStr string) error {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return err
	}
	st, err := openCLI(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.MarkReminderDone(ctx, id); err != nil {
		return err
	}
	fmt.Fprintln(w, render.Success(scope, "done"))
	return nil
}

func CLICatchUp(ctx context.Context, w io.Writer, scope *ui.Scope, home, role string) error {
	if role == "" {
		role = "default"
	}
	store, err := kv.Open(ctx, config.DataPath(home, config.ServeDB))
	if err != nil {
		return err
	}
	defer store.Close()
	// Same scoping the serve/daemon path gets, so both write one watermark.
	job := ReminderJob{Home: home, Role: role, KV: plugin.ScopeKV(store, PluginID)}
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
