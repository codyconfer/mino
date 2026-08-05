package ntr

import (
	"context"
	"fmt"
	"strconv"
	"time"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/forms"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"

	"gopkg.in/yaml.v3"

	"github.com/codyconfer/mino/internal/keymap"
	"github.com/codyconfer/mino/internal/render"
	"github.com/codyconfer/mino/internal/signals"
)

const (
	recordWriteTimeout = 2 * time.Second
	recordReadTimeout  = 5 * time.Second
)

type editorShell = vkdeck.Editor

func recordKeys(sc keys.Scheme) vkdeck.EditorKeys {
	return vkdeck.EditorKeys{
		Map: keymap.Form(sc,
			sc.Binding(keymap.Run),
			sc.Binding(keymap.Validate),
			sc.Binding(keymap.Preview),
			sc.Binding(keymap.Delete),
			sc.Binding(keymap.Focus),
			sc.Binding(keymap.Copy),
		),
		Confirm:  keymap.ConfirmMap(sc),
		Run:      keymap.Run,
		Save:     keymap.Save,
		Validate: keymap.Validate,
		Preview:  keymap.Preview,
		Delete:   keymap.Delete,
		Focus:    keymap.Focus,
		Copy:     keymap.Copy,
	}
}

// recordViewDoc is the doc surface a record view exposes; recordDoc adapts it
// to vkdeck.EditorDoc, whose Context signature differs from vkdeck.View's.
type recordViewDoc interface {
	Kind() string
	Title() string
	SavedName() string
	Fields(prev map[string]any) []forms.Field
	Sync() bool
	Summary() string
	PreviewLines(f layout.Frame) []string
	ValidateLines(f layout.Frame) ([]string, error)
	Run() (string, func() vkdeck.Results, error)
	Persist() (string, error)
	Remove() (string, error)
	CopyOutput() (string, error)
	WriteOutput() (string, error)
	docCtx() []keys.Hint
}

// recordDoc adapts a record view into vkdeck.EditorDoc.
type recordDoc struct{ recordViewDoc }

func (d recordDoc) Context() []keys.Hint { return d.docCtx() }

func newRecordEditor(view recordViewDoc, seed map[string]any, sc keys.Scheme) *editorShell {
	return vkdeck.NewEditor(recordDoc{view}, recordKeys(sc), seed)
}

type recordCore struct {
	*editorShell

	home   string
	role   string
	kind   string
	id     int64
	bucket int64
	extra  []int64
	read   func() (record, error)
	copy   func(string) error
	dirty  func()
	now    func() time.Time
}

func (c *recordCore) fileNew(ctx context.Context, st *Store, id int64) error {
	for _, b := range append([]int64{c.bucket}, c.extra...) {
		if b == 0 {
			continue
		}
		if err := st.AddMember(ctx, b, id, c.kind); err != nil {
			return err
		}
	}
	return nil
}

func (c *recordCore) Kind() string { return c.kind }

func (c *recordCore) docCtx() []keys.Hint {
	ctx := []keys.Hint{{Key: "role", Label: c.role}, {Key: "notes", Label: recordScreen(c.kind)}}
	if c.id != 0 {
		ctx = append(ctx, keys.Hint{Key: "item", Label: c.label()})
	}
	return ctx
}

func (c *recordCore) SavedName() string {
	if c.id == 0 {
		return ""
	}
	return c.label()
}

func (c *recordCore) Sync() bool { return false }

func (c *recordCore) Summary() string {
	rec, err := c.read()
	if err != nil {
		return "unsaved draft"
	}
	return rec.summary()
}

func (c *recordCore) PreviewLines(f layout.Frame) []string {
	rec, err := c.read()
	if err != nil {
		return []string{f.Theme().Cant.Render(err.Error())}
	}
	data, err := yaml.Marshal(rec.preview())
	if err != nil {
		return []string{f.Theme().Cant.Render(err.Error())}
	}
	return layout.Lines(string(data))
}

func (c *recordCore) ValidateLines(f layout.Frame) ([]string, error) {
	rec, err := c.read()
	if err != nil {
		return nil, err
	}
	return rec.check(f.Theme(), c.clock()), nil
}

func (c *recordCore) Run() (string, func() vkdeck.Results, error) {
	home, role, kind := c.home, c.role, c.kind
	label := SignalName + " · " + role
	return label, func() vkdeck.Results {
		return &render.SectionResults{Sections: recordRun(home, role, kind, label)}
	}, nil
}

func (c *recordCore) Remove() (string, error) {
	if c.id == 0 {
		return "nothing to delete", nil
	}
	label := c.label()
	err := withStore(c.home, c.role, recordWriteTimeout, func(ctx context.Context, st *Store) error {
		return removeRecord(ctx, st, c.kind, c.id)
	})
	if err != nil {
		return "", fmt.Errorf("did not delete %s: %w", label, err)
	}
	c.markDirty()
	return "removed " + label + ".", nil
}

func (c *recordCore) CopyOutput() (string, error) {
	rec, err := c.read()
	if err != nil {
		return "", err
	}
	if c.copy == nil {
		return "", fmt.Errorf("no clipboard available in this build")
	}
	text := rec.text()
	if err := c.copy(text); err != nil {
		return "", fmt.Errorf("copying the %s: %w", c.kind, err)
	}
	return "copied " + strconv.Itoa(len(text)) + " bytes to the clipboard.", nil
}

func (c *recordCore) WriteOutput() (string, error) {
	return "", fmt.Errorf("a %s has no file output", c.kind)
}

func (c *recordCore) label() string { return record{Kind: c.kind, ID: c.id}.label() }

func (c *recordCore) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c *recordCore) markDirty() {
	if c.dirty != nil {
		c.dirty()
	}
}

func (c *recordCore) stored(id int64, created bool) string {
	c.id = id
	c.markDirty()
	if created {
		return "created " + c.label() + "."
	}
	return "updated " + c.label() + "."
}

func recordRun(home, role, kind, label string) []signals.Section {
	ctx, cancel := context.WithTimeout(context.Background(), recordReadTimeout)
	defer cancel()
	var (
		secs []signals.Section
		err  error
	)
	if kind == kindReminder {
		secs, err = ReminderJob{Home: home, Role: role}.Fetch(ctx)
	} else {
		secs, err = Signal{Home: home, Role: role}.Fetch(ctx)
	}
	if err != nil {
		return []signals.Section{{Signal: SignalName, Title: label, Err: err}}
	}
	return secs
}

func removeRecord(ctx context.Context, st *Store, kind string, id int64) error {
	switch kind {
	case kindTask:
		return st.DeleteTask(ctx, id)
	case kindReminder:
		return st.DeleteReminder(ctx, id)
	default:
		return st.DeleteNote(ctx, id)
	}
}

func withStore(home, role string, timeout time.Duration, fn func(context.Context, *Store) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	st, err := Open(ctx, home, role)
	if err != nil {
		return err
	}
	defer st.Close()
	return fn(ctx, st)
}
