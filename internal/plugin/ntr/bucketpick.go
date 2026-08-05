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

	"github.com/codyconfer/mino/internal/keymap"
	"github.com/codyconfer/mino/internal/render/glyph"
	"github.com/codyconfer/mino/internal/signals"
)

const (
	pickNewKey    = "new"
	pickAnchorKey = "anchor"
)

type BucketTarget struct {
	Kind   string
	Anchor string
	Name   string
	Title  string
	Body   string
}

func ItemTarget(signal string, it signals.Item) BucketTarget {
	name := strings.TrimSpace(signals.CleanLine(it.Title))
	if name == "" {
		name = strings.TrimSpace(signal)
	}
	return BucketTarget{
		Kind:   BucketKindItem,
		Anchor: strings.TrimSpace(it.URL),
		Name:   name,
		Title:  name,
		Body:   strings.TrimSpace(it.URL),
	}
}

func RunTarget(id int64, kind, name string) BucketTarget {
	label := strings.TrimSpace(kind)
	if label == "" {
		label = "run"
	}
	label += " " + strings.TrimSpace(name) + " #" + strconv.FormatInt(id, 10)
	return BucketTarget{
		Kind:   BucketKindRun,
		Anchor: RunAnchor(id),
		Name:   label,
		Title:  label,
	}
}

func (t BucketTarget) anchorLabel() string {
	if t.Kind == BucketKindRun {
		return "This run"
	}
	return "This item"
}

func NewBucketPicker(home, role string, t BucketTarget, dirty func(), sc keys.Scheme) vkdeck.View {
	if role == "" {
		role = "default"
	}
	ctx := bucketListCtx(role)
	if t.Anchor == "" {
		return vkdeck.NewMessage("nothing to anchor",
			"This row has no link to file against, so there is nothing to attach a note to.", ctx)
	}
	return newBucketPicker(home, role, t, dirty, sc)
}

type pickSet struct {
	anchor  Bucket
	filed   bool
	buckets []Bucket
	err     error
}

type bucketPicker struct {
	*vkdeck.ItemList

	home   string
	role   string
	target BucketTarget
	dirty  func()
	sc     keys.Scheme
}

func newBucketPicker(home, role string, t BucketTarget, dirty func(), sc keys.Scheme) *bucketPicker {
	v := &bucketPicker{home: home, role: role, target: t, dirty: dirty, sc: sc}
	v.ItemList = vkdeck.NewItemList(vkdeck.ItemListSpec{
		Title: "file into",
		Ctx:   bucketListCtx(role),
		Fetch: func() any { return loadPick(home, role, t) },
		Bind: func(width int, fetched any) []list.Item {
			set, _ := fetched.(pickSet)
			return pickRows(width, t, set)
		},
		ReloadHint: "refresh",
	})
	v.OnOpen = func(string) error { return nil }
	v.OnSelect = func(h *vkdeck.Model, it list.Item) tea.Cmd { return v.choose(h, it) }
	return v
}

func (v *bucketPicker) choose(h *vkdeck.Model, it list.Item) tea.Cmd {
	switch it.Key {
	case pickNewKey:
		return h.Push(v.nameForm())
	case pickAnchorKey:
		return v.resolveAnchor(h)
	}
	b, ok := it.Payload.(Bucket)
	if !ok {
		return nil
	}
	return v.pushKinds(h, b)
}

func (v *bucketPicker) nameForm() vkdeck.View {
	return vkdeck.NewFormView(vkdeck.FormSpec{
		Title: "new bucket",
		Fields: []forms.Field{
			{Key: "name", Label: "name (required to save)", Kind: forms.FieldText, Text: v.target.Name},
		},
		Keys:    vkdeck.FormKeys{Map: keymap.Form(v.sc), Save: keymap.Save},
		Context: bucketListCtx(v.role),
		OnSubmit: func(h *vkdeck.Model, vals map[string]any) tea.Cmd {
			name := strings.TrimSpace(forms.Str(vals, "name"))
			if name == "" {
				return h.Push(vkdeck.NewMessage("failed", "a bucket needs a name", bucketListCtx(v.role)))
			}
			var made Bucket
			err := withStore(v.home, v.role, recordWriteTimeout, func(ctx context.Context, st *Store) error {
				b, err := st.CreateBucket(ctx, name, BucketKindUser, "")
				made = b
				return err
			})
			if err != nil {
				return h.Push(vkdeck.NewMessage("failed", err.Error(), bucketListCtx(v.role)))
			}
			return tea.Sequence(h.Pop(), v.pushKinds(h, made))
		},
	})
}

func (v *bucketPicker) resolveAnchor(h *vkdeck.Model) tea.Cmd {
	var b Bucket
	err := withStore(v.home, v.role, recordWriteTimeout, func(ctx context.Context, st *Store) error {
		got, err := st.EnsureAnchorBucket(ctx, v.target.Kind, v.target.Anchor, v.target.Name)
		b = got
		return err
	})
	if err != nil {
		return h.Push(vkdeck.NewMessage("failed", err.Error(), bucketListCtx(v.role)))
	}
	return v.pushKinds(h, b)
}

func (v *bucketPicker) pushKinds(h *vkdeck.Model, b Bucket) tea.Cmd {
	items := make([]vkdeck.MenuItem, 0, 3)
	for _, kind := range bucketKinds() {
		items = append(items, vkdeck.MenuItem{
			Label: strings.ToUpper(kind[:1]) + kind[1:],
			Desc:  recordNewDesc(kind),
			Icon:  glyph.Builder(),
			Hue:   recordHue(kind),
			OnSelect: func(h *vkdeck.Model) tea.Cmd {
				return h.Push(v.editor(b, kind))
			},
		})
	}
	return h.Push(vkdeck.NewMenu("file into "+b.Name, bucketListCtx(v.role), items...))
}

func (v *bucketPicker) editor(b Bucket, kind string) vkdeck.View {
	rec := record{Kind: kind, Title: v.target.Title, Bucket: b.ID}
	if kind == kindNote {
		rec.Body = v.target.Body
	}
	return buildRecordViewIn(v.home, v.role, rec, v.anchorExtra(b), v.dirty, v.sc)
}

func (v *bucketPicker) anchorExtra(chosen Bucket) []int64 {
	if chosen.Kind == v.target.Kind && chosen.Anchor == v.target.Anchor {
		return nil
	}
	var anchor Bucket
	err := withStore(v.home, v.role, recordWriteTimeout, func(ctx context.Context, st *Store) error {
		got, err := st.EnsureAnchorBucket(ctx, v.target.Kind, v.target.Anchor, v.target.Name)
		anchor = got
		return err
	})
	if err != nil || anchor.ID == 0 || anchor.ID == chosen.ID {
		return nil
	}
	return []int64{anchor.ID}
}

func pickRows(width int, t BucketTarget, set pickSet) []list.Item {
	f := layout.ScreenFrame(max(width-recordIndent, 1))
	th := f.Theme()
	items := []list.Item{{
		Block:      f.Spread(th.Icon(glyph.Builder(), bucketHue)+th.Val.Render("New bucket…"), th.Dim.Render("name one and file into it")),
		Key:        pickNewKey,
		Selectable: true,
	}}
	if set.err != nil {
		return append(items, list.Item{Block: th.Cant.Render(set.err.Error())})
	}
	items = append(items, list.Item{
		Block: f.Spread(
			th.Icon(glyph.Bucket(), bucketHue)+th.Val.Render(t.anchorLabel()),
			th.Dim.Render(anchorMeta(set))),
		Key:        pickAnchorKey,
		Selectable: true,
	})
	for _, b := range set.buckets {
		if set.filed && b.ID == set.anchor.ID {
			continue
		}
		items = append(items, list.Item{
			Block:      f.Spread(th.Val.Render(b.Name), th.Dim.Render(bucketRowMeta(b))),
			Key:        strconv.FormatInt(b.ID, 10),
			Selectable: true,
			Payload:    b,
		})
	}
	return items
}

func anchorMeta(set pickSet) string {
	if !set.filed {
		return "not filed yet"
	}
	return strconv.Itoa(set.anchor.Members) + " " + plural(set.anchor.Members, "record") + " already filed"
}

func loadPick(home, role string, t BucketTarget) pickSet {
	var set pickSet
	set.err = withStore(home, role, recordReadTimeout, func(ctx context.Context, st *Store) error {
		b, ok, err := st.BucketByAnchor(ctx, t.Kind, t.Anchor)
		if err != nil {
			return err
		}
		set.anchor, set.filed = b, ok
		bs, err := st.ListBuckets(ctx)
		if err != nil {
			return err
		}
		set.buckets = bs
		return nil
	})
	return set
}
