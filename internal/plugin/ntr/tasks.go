package ntr

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/codyconfer/viewkit/clipboard"
	"github.com/codyconfer/viewkit/forms"
	"github.com/codyconfer/viewkit/keys"
)

type taskView struct {
	recordCore

	base record
}

func newTasksList(home, role string) *recordList {
	return newRecordList(home, role, kindTask)
}

func taskRecord(t Task) record {
	return record{Kind: kindTask, ID: t.ID, Title: t.Title, Done: t.Done, Due: t.Due}
}

func newTaskView(home, role string, base record, dirty func(), sc keys.Scheme) *taskView {
	v := &taskView{base: base}
	v.recordCore = recordCore{
		home:  home,
		role:  role,
		kind:  kindTask,
		id:    base.ID,
		copy:  clipboard.Copy,
		dirty: dirty,
	}
	v.read = v.task
	v.editorShell = newRecordEditor(v, map[string]any{
		"title": base.Title,
		"due":   dueSeed(base.Due),
		"done":  base.Done,
	}, sc)
	return v
}

func (v *taskView) Title() string {
	if v.id == 0 {
		return "build task"
	}
	return "edit " + v.label()
}

func (v *taskView) Fields(prev map[string]any) []forms.Field {
	return []forms.Field{
		{Key: "title", Label: "title (required to save)", Kind: forms.FieldText, Text: forms.Raw(prev, "title")},
		{Key: "due", Label: "due (optional — RFC3339 or +2h)", Kind: forms.FieldText, Text: forms.Raw(prev, "due")},
		{Key: "done", Label: "done", Kind: forms.FieldToggle, On: forms.Bool(prev, "done")},
	}
}

func (v *taskView) task() (record, error) {
	rec := v.base
	rec.Kind = kindTask
	rec.ID = v.id
	rec.Title = strings.TrimSpace(v.Value("title"))
	rec.Done = forms.Bool(v.Form().Values(), "done")
	if rec.Title == "" {
		return record{}, fmt.Errorf("a task needs a title")
	}
	raw := v.Value("due")
	if raw == "" {
		rec.Due = time.Time{}
		return rec, nil
	}
	due, err := parseDueAt(raw, v.clock())
	if err != nil {
		return record{}, err
	}
	rec.Due = due
	return rec, nil
}

func (v *taskView) Persist() (string, error) {
	rec, err := v.task()
	if err != nil {
		return "", err
	}
	created := v.id == 0
	id := v.id
	err = withStore(v.home, v.role, recordWriteTimeout, func(ctx context.Context, st *Store) error {
		if !created {
			return st.UpdateTask(ctx, id, rec.Title, rec.Done, rec.Due)
		}
		t, err := st.CreateTask(ctx, rec.Title, rec.Due)
		if err != nil {
			return err
		}
		id = t.ID
		if !rec.Done {
			return nil
		}
		return st.SetTaskDone(ctx, id, true)
	})
	if err != nil {
		return "", err
	}
	rec.ID = id
	v.base = rec
	return v.stored(id, created), nil
}

func dueSeed(due time.Time) string {
	if due.IsZero() {
		return ""
	}
	return due.UTC().Format(time.RFC3339)
}
