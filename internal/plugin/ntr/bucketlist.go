package ntr

import (
	"context"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/forms"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/list"
	"github.com/codyconfer/viewkit/ui"

	"github.com/codyconfer/mino/internal/keymap"
	"github.com/codyconfer/mino/internal/render/glyph"
)

const (
	bucketNewKey = "new"
	bucketHue    = 1
)

func bucketListCtx(role string) []keys.Hint {
	return []keys.Hint{{Key: "role", Label: role}, {Key: "notes", Label: "Buckets"}}
}

type bucketSet struct {
	buckets []Bucket
	err     error
}

type bucketList struct {
	*vkdeck.ItemList

	home    string
	role    string
	stale   bool
	del     *keys.Map
	rename  *keys.Map
	confirm *forms.Confirm
	target  Bucket
}

func newBucketList(home, role string) *bucketList {
	if role == "" {
		role = "default"
	}
	v := &bucketList{home: home, role: role}
	v.ItemList = vkdeck.NewItemList(vkdeck.ItemListSpec{
		Title: "buckets",
		Ctx:   bucketListCtx(role),
		Fetch: func() any { return listBuckets(home, role) },
		Bind: func(width int, fetched any) []list.Item {
			set, _ := fetched.(bucketSet)
			return bucketRows(width, set.buckets, set.err)
		},
		ReloadHint: "refresh",
	})
	v.OnOpen = func(string) error { return nil }
	v.OnSelect = func(h *vkdeck.Model, it list.Item) tea.Cmd { return v.open(h, it) }
	return v
}

func (v *bucketList) maps(scope *ui.Scope) {
	if v.del == nil {
		v.del = scope.Keys.MapFor(keymap.Delete)
	}
	if v.rename == nil {
		v.rename = scope.Keys.MapFor(keymap.Rename)
	}
}

func (v *bucketList) Hints(scope *ui.Scope) []keys.Hint {
	v.maps(scope)
	return append(v.ItemList.Hints(scope),
		v.rename.Hint(keymap.Rename),
		v.del.Hint(keymap.Delete))
}

func (v *bucketList) Update(h *vkdeck.Model, msg tea.Msg) tea.Cmd {
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
		if act, ok := v.rename.Action(m.String()); ok && act == keymap.Rename {
			return v.editName(h)
		}
		if act, ok := v.del.Action(m.String()); ok && act == keymap.Delete {
			return v.ask()
		}
	}
	return v.ItemList.Update(h, msg)
}

func (v *bucketList) selectedBucket() (Bucket, bool) {
	it, ok := v.Selected()
	if !ok || it.Key == bucketNewKey {
		return Bucket{}, false
	}
	b, ok := it.Payload.(Bucket)
	return b, ok
}

func (v *bucketList) open(h *vkdeck.Model, it list.Item) tea.Cmd {
	if it.Key == bucketNewKey {
		return h.Push(v.nameForm(h.UI().Keys, Bucket{}))
	}
	b, ok := it.Payload.(Bucket)
	if !ok {
		return nil
	}
	return h.Push(newBucketView(v.home, v.role, b))
}

func (v *bucketList) editName(h *vkdeck.Model) tea.Cmd {
	b, ok := v.selectedBucket()
	if !ok {
		return nil
	}
	return h.Push(v.nameForm(h.UI().Keys, b))
}

func (v *bucketList) nameForm(sc keys.Scheme, b Bucket) vkdeck.View {
	title := "new bucket"
	if b.ID != 0 {
		title = "rename " + b.Name
	}
	return vkdeck.NewFormView(vkdeck.FormSpec{
		Title: title,
		Fields: []forms.Field{
			{Key: "name", Label: "name (required to save)", Kind: forms.FieldText, Text: b.Name},
		},
		Keys:    vkdeck.FormKeys{Map: keymap.Form(sc), Save: keymap.Save},
		Context: bucketListCtx(v.role),
		OnSubmit: func(h *vkdeck.Model, vals map[string]any) tea.Cmd {
			return v.saveName(h, b, forms.Str(vals, "name"))
		},
	})
}

func (v *bucketList) saveName(h *vkdeck.Model, b Bucket, name string) tea.Cmd {
	name = strings.TrimSpace(name)
	if name == "" {
		return h.Push(vkdeck.NewMessage("failed", "a bucket needs a name", bucketListCtx(v.role)))
	}
	err := withStore(v.home, v.role, recordWriteTimeout, func(ctx context.Context, st *Store) error {
		if b.ID == 0 {
			_, err := st.CreateBucket(ctx, name, BucketKindUser, "")
			return err
		}
		return st.RenameBucket(ctx, b.ID, name)
	})
	if err != nil {
		return h.Push(vkdeck.NewMessage("failed", err.Error(), bucketListCtx(v.role)))
	}
	v.stale = true
	return h.Pop()
}

func (v *bucketList) ask() tea.Cmd {
	b, ok := v.selectedBucket()
	if !ok {
		return nil
	}
	v.target = b
	v.confirm = &forms.Confirm{
		Title:    "delete bucket " + b.Name + "?",
		Message:  "Records stay; only the grouping goes.",
		YesLabel: "Delete",
		NoLabel:  "Keep",
	}
	return nil
}

func (v *bucketList) answer(h *vkdeck.Model, key tea.KeyMsg) tea.Cmd {
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
		return v.remove(h)
	case forms.Cancelled:
		v.confirm = nil
	}
	return nil
}

func (v *bucketList) remove(h *vkdeck.Model) tea.Cmd {
	id := v.target.ID
	err := withStore(v.home, v.role, recordWriteTimeout, func(ctx context.Context, st *Store) error {
		return st.DeleteBucket(ctx, id)
	})
	if err != nil {
		return h.Push(vkdeck.NewMessage("delete failed", err.Error(), bucketListCtx(v.role)))
	}
	return v.Reload()
}

func (v *bucketList) Body(f layout.Frame) string {
	body := v.ItemList.Body(f)
	if v.confirm == nil {
		return body
	}
	return v.confirm.Overlay(body, f.WithWidth(layout.DialogWidth(f.Width)))
}

func bucketRows(width int, bs []Bucket, err error) []list.Item {
	f := layout.ScreenFrame(max(width-recordIndent, 1))
	th := f.Theme()
	items := []list.Item{{
		Block:      f.Spread(th.Icon(glyph.Builder(), bucketHue)+th.Val.Render("New"), th.Dim.Render("name and save a new bucket")),
		Key:        bucketNewKey,
		Selectable: true,
	}}
	switch {
	case err != nil:
		return append(items, list.Item{Block: th.Cant.Render(err.Error())})
	case len(bs) == 0:
		return append(items, list.Item{Block: th.Dim.Render("(none yet)")})
	}
	for _, b := range bs {
		items = append(items, list.Item{
			Block:      f.Spread(th.Val.Render(b.Name), th.Dim.Render(bucketRowMeta(b))),
			Key:        strconv.FormatInt(b.ID, 10),
			Selectable: true,
			Payload:    b,
		})
	}
	return items
}

func bucketRowMeta(b Bucket) string {
	kind := b.Kind
	if b.Anchored() {
		kind = "anchored"
	}
	return kind + " · " + strconv.Itoa(b.Members) + " " + plural(b.Members, "record")
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

func listBuckets(home, role string) bucketSet {
	var set bucketSet
	set.err = withStore(home, role, recordReadTimeout, func(ctx context.Context, st *Store) error {
		bs, err := st.ListBuckets(ctx)
		if err != nil {
			return err
		}
		set.buckets = bs
		return nil
	})
	return set
}
