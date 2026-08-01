package ntr

import (
	"context"
	"fmt"
	"strings"

	"github.com/codyconfer/viewkit/clipboard"
	"github.com/codyconfer/viewkit/forms"
	"github.com/codyconfer/viewkit/keys"
)

const remindDueSeed = "+1h"

type remindView struct {
	recordCore

	base record
}

func newRemindersList(home, role string) *recordList {
	return newRecordList(home, role, kindReminder)
}

func remindRecord(r Reminder) record {
	return record{Kind: kindReminder, ID: r.ID, Title: r.Title, Due: r.Due, Done: r.Done}
}

func newRemindView(home, role string, base record, dirty func()) *remindView {
	v := &remindView{base: base}
	v.recordCore = recordCore{
		home:  home,
		role:  role,
		kind:  kindReminder,
		id:    base.ID,
		copy:  clipboard.Copy,
		dirty: dirty,
	}
	v.read = v.remind
	due := dueSeed(base.Due)
	if base.ID == 0 && due == "" {
		due = remindDueSeed
	}
	v.editorShell = newRecordEditor(v, map[string]any{
		"title": base.Title,
		"due":   due,
	})
	return v
}

func (v *remindView) Title() string {
	if v.id == 0 {
		return "build reminder"
	}
	return "edit " + v.label()
}

func (v *remindView) Context() []keys.Hint {
	ctx := v.recordCore.Context()
	if v.base.Done {
		return append(ctx, keys.Hint{Key: "done", Label: "yes"})
	}
	return ctx
}

func (v *remindView) Fields(prev map[string]any) []forms.Field {
	return []forms.Field{
		{Key: "title", Label: "title (required to save)", Kind: forms.FieldText, Text: forms.Raw(prev, "title")},
		{Key: "due", Label: "due (RFC3339 or +2h)", Kind: forms.FieldText, Text: forms.Raw(prev, "due")},
	}
}

func (v *remindView) remind() (record, error) {
	rec := v.base
	rec.Kind = kindReminder
	rec.ID = v.id
	rec.Title = strings.TrimSpace(v.Value("title"))
	if rec.Title == "" {
		return record{}, fmt.Errorf("a reminder needs a title")
	}
	raw := v.Value("due")
	if raw == "" {
		return record{}, fmt.Errorf("due: a reminder with no due never fires")
	}
	due, err := parseDueAt(raw, v.clock())
	if err != nil {
		return record{}, err
	}
	rec.Due = due
	return rec, nil
}

func (v *remindView) Persist() (string, error) {
	rec, err := v.remind()
	if err != nil {
		return "", err
	}
	created := v.id == 0
	id := v.id
	err = withStore(v.home, v.role, recordWriteTimeout, func(ctx context.Context, st *Store) error {
		if !created {
			return st.UpdateReminder(ctx, id, rec.Title, rec.Due)
		}
		r, err := st.CreateReminder(ctx, rec.Title, rec.Due)
		if err != nil {
			return err
		}
		id = r.ID
		return nil
	})
	if err != nil {
		return "", err
	}
	rec.ID = id
	v.base = rec
	return v.stored(id, created), nil
}
