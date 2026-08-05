package ntr

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
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
	return CLINotesAddIn(ctx, w, scope, home, role, title, body, "")
}

func CLINotesAddIn(ctx context.Context, w io.Writer, scope *ui.Scope, home, role, title, body, bucketStr string) error {
	st, err := openCLI(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	n, err := st.CreateNote(ctx, title, body)
	if err != nil {
		return err
	}
	filed, err := cliFile(ctx, st, bucketStr, n.ID, kindNote)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, render.Success(scope, fmt.Sprintf("note %d created%s", n.ID, filed)))
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
	return CLITasksAddIn(ctx, w, scope, home, role, title, "")
}

func CLITasksAddIn(ctx context.Context, w io.Writer, scope *ui.Scope, home, role, title, bucketStr string) error {
	st, err := openCLI(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	t, err := st.CreateTask(ctx, title, time.Time{})
	if err != nil {
		return err
	}
	filed, err := cliFile(ctx, st, bucketStr, t.ID, kindTask)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, render.Success(scope, fmt.Sprintf("task %d created%s", t.ID, filed)))
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
	return CLIRemindAddIn(ctx, w, scope, home, role, title, dur, "")
}

func CLIRemindAddIn(ctx context.Context, w io.Writer, scope *ui.Scope, home, role, title, dur, bucketStr string) error {
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
	filed, err := cliFile(ctx, st, bucketStr, r.ID, kindReminder)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, render.Success(scope, fmt.Sprintf("reminder %d at %s%s", r.ID, r.Due.Format(time.RFC3339), filed)))
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

func cliFile(ctx context.Context, st *Store, bucketStr string, id int64, kind string) (string, error) {
	if strings.TrimSpace(bucketStr) == "" {
		return "", nil
	}
	bucket, err := strconv.ParseInt(strings.TrimSpace(bucketStr), 10, 64)
	if err != nil {
		return "", fmt.Errorf("--bucket must be a bucket id: %w", err)
	}
	if err := st.AddMember(ctx, bucket, id, kind); err != nil {
		return "", err
	}
	return fmt.Sprintf(" and filed into bucket %d", bucket), nil
}

func CLIBucketsList(ctx context.Context, w io.Writer, scope *ui.Scope, home, role string) error {
	st, err := openCLI(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	bs, err := st.ListBuckets(ctx)
	if err != nil {
		return err
	}
	for _, b := range bs {
		kind := b.Kind
		if b.Anchored() {
			kind += " " + b.Anchor
		}
		fmt.Fprintf(w, "%d\t%d\t%s\t%s\n", b.ID, b.Members, kind, b.Name)
	}
	return nil
}

func CLIBucketsShow(ctx context.Context, w io.Writer, scope *ui.Scope, home, role, idStr string) error {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return err
	}
	st, err := openCLI(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	b, ok, err := st.Bucket(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("bucket #%d no longer exists", id)
	}
	fmt.Fprintf(w, "%s\t%s\n", b.Name, b.Kind)
	if b.Anchored() {
		fmt.Fprintf(w, "anchor\t%s\n", b.Anchor)
	}
	recs, err := st.bucketRecords(ctx, id)
	if err != nil {
		return err
	}
	for _, rec := range recs {
		fmt.Fprintf(w, "%d\t%s\t%s\n", rec.ID, rec.Kind, rec.Title)
	}
	return nil
}

func CLIBucketsAdd(ctx context.Context, w io.Writer, scope *ui.Scope, home, role, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("a bucket needs a name")
	}
	st, err := openCLI(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	b, err := st.CreateBucket(ctx, name, BucketKindUser, "")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, render.Success(scope, fmt.Sprintf("bucket %d created", b.ID)))
	return nil
}

func CLIBucketsRename(ctx context.Context, w io.Writer, scope *ui.Scope, home, role, idStr, name string) error {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("a bucket needs a name")
	}
	st, err := openCLI(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.RenameBucket(ctx, id, name); err != nil {
		return err
	}
	fmt.Fprintln(w, render.Success(scope, fmt.Sprintf("bucket %d renamed", id)))
	return nil
}

func CLIBucketsRM(ctx context.Context, w io.Writer, scope *ui.Scope, home, role, idStr string) error {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return err
	}
	st, err := openCLI(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.DeleteBucket(ctx, id); err != nil {
		return err
	}
	fmt.Fprintln(w, render.Success(scope, "deleted; records kept"))
	return nil
}

func CLIBucketsFile(ctx context.Context, w io.Writer, scope *ui.Scope, home, role, bucketStr, idStr string) error {
	bucket, err := strconv.ParseInt(bucketStr, 10, 64)
	if err != nil {
		return err
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return err
	}
	st, err := openCLI(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	kind, ok, err := st.recordKind(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no note, task or reminder with id %d", id)
	}
	if err := st.AddMember(ctx, bucket, id, kind); err != nil {
		return err
	}
	fmt.Fprintln(w, render.Success(scope, fmt.Sprintf("%s %d filed into bucket %d", kind, id, bucket)))
	return nil
}

func CLIBucketsUnfile(ctx context.Context, w io.Writer, scope *ui.Scope, home, role, bucketStr, idStr string) error {
	bucket, err := strconv.ParseInt(bucketStr, 10, 64)
	if err != nil {
		return err
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return err
	}
	st, err := openCLI(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.RemoveMember(ctx, bucket, id); err != nil {
		return err
	}
	fmt.Fprintln(w, render.Success(scope, "unfiled; the record is kept"))
	return nil
}

func CLIBucketsFor(ctx context.Context, w io.Writer, scope *ui.Scope, home, role, idStr string) error {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return err
	}
	st, err := openCLI(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	bs, err := st.BucketsFor(ctx, id)
	if err != nil {
		return err
	}
	for _, b := range bs {
		fmt.Fprintf(w, "%d\t%s\t%s\n", b.ID, b.Kind, b.Name)
	}
	return nil
}
