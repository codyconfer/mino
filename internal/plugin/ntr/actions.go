package ntr

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/plugin"
)

func init() {
	plugin.RegisterDataPath(func(home string) string {
		return config.DataPath(home, dbName)
	})
	plugin.RegisterAction(SignalName, "note.add", actionNoteAdd)
	plugin.RegisterAction(SignalName, "note.update", actionNoteUpdate)
	plugin.RegisterAction(SignalName, "note.rm", actionNoteRM)
	plugin.RegisterAction(SignalName, "task.add", actionTaskAdd)
	plugin.RegisterAction(SignalName, "task.done", actionTaskDone)
	plugin.RegisterAction(SignalName, "task.undo", actionTaskUndo)
	plugin.RegisterAction(SignalName, "task.rm", actionTaskRM)
	plugin.RegisterAction(SignalName, "remind.add", actionRemindAdd, plugin.WithServiceOnly())
	plugin.RegisterAction(SignalName, "remind.done", actionRemindDone, plugin.WithServiceOnly())
	plugin.RegisterAction(SignalName, "bucket.add", actionBucketAdd)
	plugin.RegisterAction(SignalName, "bucket.rename", actionBucketRename)
	plugin.RegisterAction(SignalName, "bucket.rm", actionBucketRM)
	plugin.RegisterAction(SignalName, "bucket.file", actionBucketFile)
	plugin.RegisterAction(SignalName, "bucket.unfile", actionBucketUnfile)
}

func actionBucketAdd(ctx context.Context, params map[string]string) error {
	home, role := actionHomeRole(params)
	name := strings.TrimSpace(params["name"])
	if home == "" || name == "" {
		return fmt.Errorf("home and name params required")
	}
	st, err := Open(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	_, err = st.CreateBucket(ctx, name, BucketKindUser, "")
	return err
}

func actionBucketRename(ctx context.Context, params map[string]string) error {
	home, role := actionHomeRole(params)
	id, err := strconv.ParseInt(params["id"], 10, 64)
	name := strings.TrimSpace(params["name"])
	if err != nil || home == "" || name == "" {
		return fmt.Errorf("home, id and name params required")
	}
	st, err := Open(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	return st.RenameBucket(ctx, id, name)
}

func actionBucketRM(ctx context.Context, params map[string]string) error {
	home, role := actionHomeRole(params)
	id, err := strconv.ParseInt(params["id"], 10, 64)
	if err != nil || home == "" {
		return fmt.Errorf("home and id params required")
	}
	st, err := Open(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	return st.DeleteBucket(ctx, id)
}

func actionBucketFile(ctx context.Context, params map[string]string) error {
	home, role := actionHomeRole(params)
	id, err := strconv.ParseInt(params["id"], 10, 64)
	if err != nil || home == "" {
		return fmt.Errorf("home and id params required")
	}
	st, err := Open(ctx, home, role)
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
	bucket, err := resolveActionBucket(ctx, st, params)
	if err != nil {
		return err
	}
	return st.AddMember(ctx, bucket, id, kind)
}

func actionBucketUnfile(ctx context.Context, params map[string]string) error {
	home, role := actionHomeRole(params)
	id, err := strconv.ParseInt(params["id"], 10, 64)
	bucket, berr := strconv.ParseInt(params["bucket"], 10, 64)
	if err != nil || berr != nil || home == "" {
		return fmt.Errorf("home, bucket and id params required")
	}
	st, err := Open(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	return st.RemoveMember(ctx, bucket, id)
}

func resolveActionBucket(ctx context.Context, st *Store, params map[string]string) (int64, error) {
	if raw := strings.TrimSpace(params["bucket"]); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("bucket param must be a bucket id: %w", err)
		}
		return id, nil
	}
	anchor := strings.TrimSpace(params["anchor"])
	if anchor == "" {
		return 0, fmt.Errorf("either a bucket or an anchor param is required")
	}
	kind := strings.TrimSpace(params["anchor_kind"])
	if kind == "" {
		kind = BucketKindItem
	}
	if kind != BucketKindItem && kind != BucketKindRun {
		return 0, fmt.Errorf("anchor_kind must be %q or %q", BucketKindItem, BucketKindRun)
	}
	b, err := st.EnsureAnchorBucket(ctx, kind, anchor, params["name"])
	if err != nil {
		return 0, err
	}
	return b.ID, nil
}

func actionFileNew(ctx context.Context, st *Store, params map[string]string, id int64, kind string) error {
	if strings.TrimSpace(params["bucket"]) == "" && strings.TrimSpace(params["anchor"]) == "" {
		return nil
	}
	bucket, err := resolveActionBucket(ctx, st, params)
	if err != nil {
		return err
	}
	return st.AddMember(ctx, bucket, id, kind)
}

func actionHomeRole(params map[string]string) (home, role string) {
	home = params["home"]
	role = params["role"]
	if role == "" {
		role = "default"
	}
	return home, role
}

func actionNoteAdd(ctx context.Context, params map[string]string) error {
	home, role := actionHomeRole(params)
	if home == "" {
		return fmt.Errorf("home param required")
	}
	title := params["title"]
	if title == "" {
		return fmt.Errorf("title param required")
	}
	st, err := Open(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	n, err := st.CreateNote(ctx, title, params["body"])
	if err != nil {
		return err
	}
	return actionFileNew(ctx, st, params, n.ID, kindNote)
}

func actionNoteUpdate(ctx context.Context, params map[string]string) error {
	home, role := actionHomeRole(params)
	id, err := strconv.ParseInt(params["id"], 10, 64)
	if err != nil || home == "" {
		return fmt.Errorf("home and id params required")
	}
	title := params["title"]
	if title == "" {
		return fmt.Errorf("title param required")
	}
	st, err := Open(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	return st.UpdateNote(ctx, id, title, params["body"])
}

func actionNoteRM(ctx context.Context, params map[string]string) error {
	home, role := actionHomeRole(params)
	id, err := strconv.ParseInt(params["id"], 10, 64)
	if err != nil || home == "" {
		return fmt.Errorf("home and id params required")
	}
	st, err := Open(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	return st.DeleteNote(ctx, id)
}

func actionTaskAdd(ctx context.Context, params map[string]string) error {
	home, role := actionHomeRole(params)
	if home == "" || params["title"] == "" {
		return fmt.Errorf("home and title params required")
	}
	st, err := Open(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	tk, err := st.CreateTask(ctx, params["title"], time.Time{})
	if err != nil {
		return err
	}
	return actionFileNew(ctx, st, params, tk.ID, kindTask)
}

func actionTaskDone(ctx context.Context, params map[string]string) error {
	home, role := actionHomeRole(params)
	id, err := strconv.ParseInt(params["id"], 10, 64)
	if err != nil || home == "" {
		return fmt.Errorf("home and id params required")
	}
	st, err := Open(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	return st.SetTaskDone(ctx, id, true)
}

func actionTaskUndo(ctx context.Context, params map[string]string) error {
	home, role := actionHomeRole(params)
	id, err := strconv.ParseInt(params["id"], 10, 64)
	if err != nil || home == "" {
		return fmt.Errorf("home and id params required")
	}
	st, err := Open(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	return st.SetTaskDone(ctx, id, false)
}

func actionTaskRM(ctx context.Context, params map[string]string) error {
	home, role := actionHomeRole(params)
	id, err := strconv.ParseInt(params["id"], 10, 64)
	if err != nil || home == "" {
		return fmt.Errorf("home and id params required")
	}
	st, err := Open(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	return st.DeleteTask(ctx, id)
}

func actionRemindAdd(ctx context.Context, params map[string]string) error {
	home, role := actionHomeRole(params)
	if home == "" || params["title"] == "" {
		return fmt.Errorf("home and title params required")
	}
	d, err := time.ParseDuration(params["in"])
	if err != nil {
		return fmt.Errorf("in param must be a duration (e.g. 10m): %w", err)
	}
	st, err := Open(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	rm, err := st.CreateReminder(ctx, params["title"], time.Now().Add(d))
	if err != nil {
		return err
	}
	return actionFileNew(ctx, st, params, rm.ID, kindReminder)
}

func actionRemindDone(ctx context.Context, params map[string]string) error {
	home, role := actionHomeRole(params)
	id, err := strconv.ParseInt(params["id"], 10, 64)
	if err != nil || home == "" {
		return fmt.Errorf("home and id params required")
	}
	st, err := Open(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	return st.MarkReminderDone(ctx, id)
}
