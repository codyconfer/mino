package ntr

import (
	"context"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/list"
	"github.com/codyconfer/viewkit/theme"
	"github.com/codyconfer/viewkit/timefmt"

	"github.com/codyconfer/munin/internal/keymap"
	"github.com/codyconfer/munin/internal/render/glyph"
)

const (
	recordNewKey = "new"
	recordIndent = 2
)

func recordScreen(kind string) string {
	switch kind {
	case kindTask:
		return "Tasks"
	case kindReminder:
		return "Reminders"
	default:
		return "Notes"
	}
}

func recordListTitle(kind string) string {
	switch kind {
	case kindTask:
		return "tasks"
	case kindReminder:
		return "reminders"
	default:
		return "notes"
	}
}

func recordHue(kind string) int {
	switch kind {
	case kindTask:
		return 2
	case kindReminder:
		return 4
	default:
		return 6
	}
}

func recordNewDesc(kind string) string {
	switch kind {
	case kindTask:
		return "compose and save a new task"
	case kindReminder:
		return "compose and save a new reminder"
	default:
		return "compose and save a new note"
	}
}

func recordListCtx(role, kind string) [][2]string {
	return [][2]string{{"role", role}, {"notes", recordScreen(kind)}}
}

type recordSet struct {
	recs []record
	err  error
}

type recordList struct {
	*vkdeck.ItemList

	home   string
	role   string
	kind   string
	rows   map[string]record
	toggle *keys.Map
	stale  bool
}

func newRecordList(home, role, kind string) *recordList {
	v := &recordList{home: home, role: role, kind: kind, rows: map[string]record{}}
	v.ItemList = vkdeck.NewItemList(recordListTitle(kind), recordListCtx(role, kind),
		func() any { return listRecords(home, role, kind) },
		func(width int, fetched any) []list.Item {
			set, _ := fetched.(recordSet)
			v.rows = recordIndex(set.recs)
			return recordRows(width, kind, set.recs, set.err)
		},
	)
	v.ReloadHint = "refresh"
	v.OnOpen = func(string) error { return nil }
	v.OnSelect = func(h *vkdeck.Model, key string) tea.Cmd { return v.open(h, key) }
	if kind != kindNote {
		v.toggle = keys.NewMap(keymap.ToggleBinding())
	}
	return v
}

func (v *recordList) Hints() [][2]string {
	nav := recordListKeys()
	hints := [][2]string{
		nav.HintLabeled(keys.Up, "row"),
		nav.HintLabeled(keys.Confirm, "edit"),
	}
	if v.toggle != nil {
		hints = append(hints, v.toggle.HintLabeled(keymap.Toggle, recordToggleLabel(v.kind)))
	}
	return append(hints,
		nav.HintLabeled(keys.PageUp, "page"),
		nav.HintLabeled(keys.Reload, "refresh"),
	)
}

func (v *recordList) Update(h *vkdeck.Model, msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		cmd := v.ItemList.Update(h, msg)
		if v.stale {
			v.stale = false
			return tea.Batch(cmd, v.Reload())
		}
		return cmd
	case tea.KeyMsg:
		if v.toggle != nil {
			if act, ok := v.toggle.Action(m.String()); ok && act == keymap.Toggle {
				return v.setDone(h)
			}
		}
	}
	return v.ItemList.Update(h, msg)
}

func (v *recordList) open(h *vkdeck.Model, key string) tea.Cmd {
	if key == recordNewKey {
		return h.Push(v.build(record{Kind: v.kind}))
	}
	rec, ok := v.rows[key]
	if !ok {
		return nil
	}
	return h.Push(v.build(rec))
}

func (v *recordList) build(rec record) vkdeck.View {
	switch v.kind {
	case kindTask:
		return newTaskView(v.home, v.role, rec, v.markStale)
	case kindReminder:
		return newRemindView(v.home, v.role, rec, v.markStale)
	default:
		return newNoteView(v.home, v.role, rec, v.markStale)
	}
}

func (v *recordList) setDone(h *vkdeck.Model) tea.Cmd {
	it, ok := v.Selected()
	if !ok || it.Key == "" || it.Key == recordNewKey {
		return nil
	}
	rec, ok := v.rows[it.Key]
	if !ok {
		return nil
	}
	err := withStore(v.home, v.role, recordWriteTimeout, func(ctx context.Context, st *Store) error {
		if v.kind == kindReminder {
			return st.MarkReminderDone(ctx, rec.ID)
		}
		return st.SetTaskDone(ctx, rec.ID, !rec.Done)
	})
	if err != nil {
		return h.Push(vkdeck.NewMessage("failed", err.Error(), v.Context()))
	}
	return v.Reload()
}

func (v *recordList) markStale() { v.stale = true }

func recordListKeys() *keys.Map {
	sc := keys.Cur()
	return keys.NewMap(
		sc.Binding(keys.Up),
		sc.Binding(keys.Down),
		sc.Binding(keys.PageUp),
		sc.Binding(keys.PageDown),
		sc.Binding(keys.Confirm),
		sc.Binding(keys.Reload),
		sc.Binding(keys.Cancel),
	)
}

func recordToggleLabel(kind string) string {
	if kind == kindReminder {
		return "done"
	}
	return "toggle"
}

func recordIndex(recs []record) map[string]record {
	out := make(map[string]record, len(recs))
	for _, rec := range recs {
		out[strconv.FormatInt(rec.ID, 10)] = rec
	}
	return out
}

func recordRows(width int, kind string, recs []record, err error) []list.Item {
	th := theme.Cur()
	f := layout.ScreenFrame(max(width-recordIndent, 1))
	items := []list.Item{{
		Block:      f.Spread(theme.Icon(glyph.Builder(), recordHue(kind))+th.Val.Render("New"), th.Dim.Render(recordNewDesc(kind))),
		Key:        recordNewKey,
		Selectable: true,
	}}
	switch {
	case err != nil:
		return append(items, list.Item{Block: th.Cant.Render(err.Error())})
	case len(recs) == 0:
		return append(items, list.Item{Block: th.Dim.Render("(none yet)")})
	}
	for _, rec := range recs {
		items = append(items, list.Item{
			Block:      f.Spread(th.Val.Render(recordRowLabel(rec)), th.Dim.Render(recordRowMeta(rec))),
			Key:        strconv.FormatInt(rec.ID, 10),
			Selectable: true,
		})
	}
	return items
}

func recordRowLabel(rec record) string {
	if rec.Kind == kindNote {
		return rec.Title
	}
	mark := "[ ] "
	if rec.Done {
		mark = "[x] "
	}
	return mark + rec.Title
}

func recordRowMeta(rec record) string {
	if rec.Kind == kindNote {
		if rec.Body == "" {
			return "no body"
		}
		return strconv.Itoa(len(rec.Body)) + " chars"
	}
	if rec.Due.IsZero() {
		return "no due"
	}
	return dueStamp(rec.Due) + "  " + timefmt.Rel(rec.Due)
}

func listRecords(home, role, kind string) recordSet {
	var set recordSet
	set.err = withStore(home, role, recordReadTimeout, func(ctx context.Context, st *Store) error {
		recs, err := fetchRecords(ctx, st, kind)
		if err != nil {
			return err
		}
		set.recs = recs
		return nil
	})
	return set
}

func fetchRecords(ctx context.Context, st *Store, kind string) ([]record, error) {
	switch kind {
	case kindTask:
		tasks, err := st.ListTasks(ctx, true)
		if err != nil {
			return nil, err
		}
		out := make([]record, 0, len(tasks))
		for _, t := range tasks {
			out = append(out, taskRecord(t))
		}
		return out, nil
	case kindReminder:
		items, err := st.ListReminders(ctx, false)
		if err != nil {
			return nil, err
		}
		out := make([]record, 0, len(items))
		for _, r := range items {
			out = append(out, remindRecord(r))
		}
		return out, nil
	default:
		notes, err := st.ListNotes(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]record, 0, len(notes))
		for _, n := range notes {
			out = append(out, noteRecord(n))
		}
		return out, nil
	}
}
