package ntr

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/forms"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/list"
	"github.com/codyconfer/viewkit/theme"
	"github.com/codyconfer/viewkit/ui"

	"github.com/codyconfer/mino/internal/keymap"
	"github.com/codyconfer/mino/internal/plugin"
	"github.com/codyconfer/mino/internal/render/glyph"
)

const bucketNewPrefix = "new:"

type bucketView struct {
	*vkdeck.ItemList

	home    string
	role    string
	bucket  Bucket
	stale   bool
	del     *keys.Map
	toggle  *keys.Map
	confirm *forms.Confirm
	target  record
}

func bucketViewCtx(role string, b Bucket) []keys.Hint {
	ctx := []keys.Hint{{Key: "role", Label: role}, {Key: "bucket", Label: b.Name}}
	if b.Anchored() {
		ctx = append(ctx, keys.Hint{Key: "kind", Label: b.Kind})
	}
	return ctx
}

func newBucketView(home, role string, b Bucket) *bucketView {
	if role == "" {
		role = "default"
	}
	v := &bucketView{home: home, role: role, bucket: b}
	v.ItemList = vkdeck.NewItemList(vkdeck.ItemListSpec{
		Title: "bucket: " + b.Name,
		Ctx:   bucketViewCtx(role, b),
		Fetch: func() any { return listBucketRecords(home, role, b.ID) },
		Bind: func(width int, fetched any) []list.Item {
			set, _ := fetched.(recordSet)
			return bucketMemberRows(width, set.recs, set.err)
		},
		ReloadHint: "refresh",
	})
	v.OnOpen = func(string) error { return nil }
	v.OnSelect = func(h *vkdeck.Model, it list.Item) tea.Cmd { return v.open(h, it) }
	return v
}

func (v *bucketView) maps(scope *ui.Scope) {
	if v.del == nil {
		v.del = scope.Keys.MapFor(keymap.Delete)
	}
	if v.toggle == nil {
		v.toggle = scope.Keys.MapFor(keymap.Toggle)
	}
}

func (v *bucketView) Hints(scope *ui.Scope) []keys.Hint {
	v.maps(scope)
	return append(v.ItemList.Hints(scope),
		v.toggle.HintLabeled(keymap.Toggle, "toggle"),
		v.del.HintLabeled(keymap.Delete, "unfile"))
}

func (v *bucketView) Update(h *vkdeck.Model, msg tea.Msg) tea.Cmd {
	v.maps(h.UI())
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		cmd := v.ItemList.Update(h, msg)
		if v.stale {
			v.stale = false
			return tea.Batch(cmd, v.Reload())
		}
		return cmd
	case tea.KeyMsg:
		if v.confirm != nil {
			return v.answer(h, m)
		}
		if act, ok := v.toggle.Action(m.String()); ok && act == keymap.Toggle {
			return v.setDone(h)
		}
		if act, ok := v.del.Action(m.String()); ok && act == keymap.Delete {
			return v.ask()
		}
	}
	return v.ItemList.Update(h, msg)
}

func (v *bucketView) selectedRecord() (record, bool) {
	it, ok := v.Selected()
	if !ok {
		return record{}, false
	}
	rec, ok := it.Payload.(record)
	return rec, ok
}

func (v *bucketView) open(h *vkdeck.Model, it list.Item) tea.Cmd {
	if kind, ok := newRowKind(it.Key); ok {
		return h.Push(buildRecordView(v.home, v.role,
			record{Kind: kind, Bucket: v.bucket.ID}, v.markStale, h.UI().Keys))
	}
	rec, ok := it.Payload.(record)
	if !ok {
		return nil
	}
	return h.Push(buildRecordView(v.home, v.role, rec, v.markStale, h.UI().Keys))
}

func (v *bucketView) setDone(h *vkdeck.Model) tea.Cmd {
	rec, ok := v.selectedRecord()
	if !ok || rec.Kind == kindNote {
		return nil
	}
	err := withStore(v.home, v.role, recordWriteTimeout, func(ctx context.Context, st *Store) error {
		return setRecordDone(ctx, st, rec)
	})
	if err != nil {
		return h.Push(vkdeck.NewMessage("failed", err.Error(), bucketViewCtx(v.role, v.bucket)))
	}
	return v.Reload()
}

func (v *bucketView) ask() tea.Cmd {
	rec, ok := v.selectedRecord()
	if !ok {
		return nil
	}
	v.target = rec
	v.confirm = &forms.Confirm{
		Title:    "unfile " + rec.label() + "?",
		Message:  "It leaves this bucket. The " + rec.Kind + " itself is kept.",
		YesLabel: "Unfile",
		NoLabel:  "Keep",
	}
	return nil
}

func (v *bucketView) answer(h *vkdeck.Model, key tea.KeyMsg) tea.Cmd {
	act, ok := keymap.ConfirmMap(h.UI().Keys).Action(key.String())
	if !ok {
		return nil
	}
	switch v.confirm.Handle(act) {
	case forms.Submitted:
		yes := v.confirm.Yes
		v.confirm = nil
		if !yes {
			return nil
		}
		return v.unfile(h)
	case forms.Cancelled:
		v.confirm = nil
	}
	return nil
}

func (v *bucketView) unfile(h *vkdeck.Model) tea.Cmd {
	id := v.target.ID
	err := withStore(v.home, v.role, recordWriteTimeout, func(ctx context.Context, st *Store) error {
		return st.RemoveMember(ctx, v.bucket.ID, id)
	})
	if err != nil {
		return h.Push(vkdeck.NewMessage("unfile failed", err.Error(), bucketViewCtx(v.role, v.bucket)))
	}
	return v.Reload()
}

func (v *bucketView) Body(f layout.Frame) string {
	body := v.ItemList.Body(f)
	if v.confirm == nil {
		return body
	}
	return v.confirm.Overlay(body, f.WithWidth(layout.DialogWidth(f.Width)))
}

func (v *bucketView) markStale() { v.stale = true }

func newRowKind(key string) (string, bool) {
	kind, ok := strings.CutPrefix(key, bucketNewPrefix)
	return kind, ok && kind != ""
}

func bucketKinds() []string {
	kinds := []string{kindNote, kindTask}
	if plugin.ViewUIVisible(remindersViewID) {
		kinds = append(kinds, kindReminder)
	}
	return kinds
}

func bucketMemberRows(width int, recs []record, err error) []list.Item {
	f := layout.ScreenFrame(max(width-recordIndent, 1))
	th := f.Theme()
	var items []list.Item
	for _, kind := range bucketKinds() {
		items = append(items, list.Item{
			Block: f.Spread(
				th.Icon(glyph.Builder(), recordHue(kind))+th.Val.Render("New "+kind),
				th.Dim.Render(recordNewDesc(kind))),
			Key:        bucketNewPrefix + kind,
			Selectable: true,
		})
	}
	switch {
	case err != nil:
		return append(items, list.Item{Block: th.Cant.Render(err.Error())})
	case len(recs) == 0:
		return append(items, list.Item{Block: th.Dim.Render("(nothing filed yet)")})
	}
	for _, rec := range recs {
		items = append(items, bucketMemberRow(f, th, rec))
	}
	return items
}

func bucketMemberRow(f layout.Frame, th theme.Theme, rec record) list.Item {
	it := recordRow(f, th, rec)
	it.Block = f.Spread(
		th.Val.Render(recordRowLabel(rec)),
		th.Dim.Render(rec.Kind+" · "+recordRowMeta(rec)))
	return it
}

func listBucketRecords(home, role string, bucketID int64) recordSet {
	var set recordSet
	set.err = withStore(home, role, recordReadTimeout, func(ctx context.Context, st *Store) error {
		recs, err := st.bucketRecords(ctx, bucketID)
		if err != nil {
			return err
		}
		set.recs = recs
		return nil
	})
	return set
}
