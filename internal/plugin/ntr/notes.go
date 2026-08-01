package ntr

import (
	"context"
	"fmt"
	"strings"

	"github.com/codyconfer/viewkit/clipboard"
	"github.com/codyconfer/viewkit/forms"
	"github.com/codyconfer/viewkit/keys"
)

type noteView struct {
	recordCore

	base record
}

func newNotesList(home, role string) *recordList {
	return newRecordList(home, role, kindNote)
}

func noteRecord(n Note) record {
	return record{Kind: kindNote, ID: n.ID, Title: n.Title, Body: n.Body}
}

func newNoteView(home, role string, base record, dirty func(), sc keys.Scheme) *noteView {
	v := &noteView{base: base}
	v.recordCore = recordCore{
		home:  home,
		role:  role,
		kind:  kindNote,
		id:    base.ID,
		copy:  clipboard.Copy,
		dirty: dirty,
	}
	v.read = v.note
	v.editorShell = newRecordEditor(v, map[string]any{
		"title": base.Title,
		"body":  base.Body,
	}, sc)
	return v
}

func (v *noteView) Title() string {
	if v.id == 0 {
		return "build note"
	}
	return "edit " + v.label()
}

func (v *noteView) Fields(prev map[string]any) []forms.Field {
	return []forms.Field{
		{Key: "title", Label: "title (required to save)", Kind: forms.FieldText, Text: forms.Raw(prev, "title")},
		{Key: "body", Label: "body", Kind: forms.FieldMultiline, Text: forms.Raw(prev, "body")},
	}
}

func (v *noteView) note() (record, error) {
	rec := v.base
	rec.Kind = kindNote
	rec.ID = v.id
	rec.Title = strings.TrimSpace(v.Value("title"))
	rec.Body = forms.Raw(v.Form().Values(), "body")
	if rec.Title == "" {
		return record{}, fmt.Errorf("a note needs a title")
	}
	return rec, nil
}

func (v *noteView) Persist() (string, error) {
	rec, err := v.note()
	if err != nil {
		return "", err
	}
	created := v.id == 0
	id := v.id
	err = withStore(v.home, v.role, recordWriteTimeout, func(ctx context.Context, st *Store) error {
		if !created {
			return st.UpdateNote(ctx, id, rec.Title, rec.Body)
		}
		n, err := st.CreateNote(ctx, rec.Title, rec.Body)
		if err != nil {
			return err
		}
		id = n.ID
		return nil
	})
	if err != nil {
		return "", err
	}
	rec.ID = id
	v.base = rec
	return v.stored(id, created), nil
}
