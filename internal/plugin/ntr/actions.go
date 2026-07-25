package ntr

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/plugin"
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
	plugin.RegisterAction(SignalName, "remind.add", actionRemindAdd)
	plugin.RegisterAction(SignalName, "remind.done", actionRemindDone)
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
	_, err = st.CreateNote(ctx, title, params["body"])
	return err
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
	_, err = st.CreateTask(ctx, params["title"], time.Time{})
	return err
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
	_, err = st.CreateReminder(ctx, params["title"], time.Now().Add(d))
	return err
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
