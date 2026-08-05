package ntr

import (
	"context"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/mino/internal/deck"
)

func loadedBucketList(t *testing.T, v *bucketList) *vkdeck.Model {
	t.Helper()
	app := deck.New(v, deck.WithScope(testScope()))
	app = step(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	return settle(app, v.Init())
}

func TestBucketRowsPutNewFirst(t *testing.T) {
	bs := []Bucket{
		{ID: 3, Name: "escalations", Kind: BucketKindUser, Members: 2},
		{ID: 7, Name: "PR #1", Kind: BucketKindItem, Anchor: "https://x/1", Members: 1},
	}
	items := bucketRows(80, bs, nil)
	if len(items) != 3 {
		t.Fatalf("rows = %d, want New plus two buckets", len(items))
	}
	if items[0].Key != bucketNewKey || !items[0].Selectable {
		t.Fatalf("first row = %+v, want a selectable New row", items[0])
	}
	for i, b := range bs {
		row := items[i+1]
		if want := strconv.FormatInt(b.ID, 10); row.Key != want {
			t.Errorf("row %d key = %q, want %q", i+1, row.Key, want)
		}
		if !strings.Contains(row.Block, b.Name) {
			t.Errorf("row %d = %q, want the name", i+1, row.Block)
		}
		if got, ok := row.Payload.(Bucket); !ok || got.ID != b.ID {
			t.Errorf("row %d payload = %+v, want the bucket", i+1, row.Payload)
		}
	}
	if !strings.Contains(items[1].Block, "2 records") {
		t.Errorf("user row = %q, want a plural member count", items[1].Block)
	}
	if !strings.Contains(items[2].Block, "anchored") {
		t.Errorf("item row = %q, want it labelled anchored", items[2].Block)
	}
	if !strings.Contains(items[2].Block, "1 record") || strings.Contains(items[2].Block, "1 records") {
		t.Errorf("item row = %q, want a singular member count", items[2].Block)
	}
}

func TestBucketRowsEmptyAndError(t *testing.T) {
	items := bucketRows(80, nil, nil)
	if len(items) != 2 || !strings.Contains(items[1].Block, "(none yet)") {
		t.Fatalf("empty rows = %+v, want New plus a none-yet row", items)
	}
	if items[1].Selectable {
		t.Error("the none-yet row is selectable")
	}
	items = bucketRows(80, nil, context.DeadlineExceeded)
	if len(items) != 2 || !strings.Contains(items[1].Block, context.DeadlineExceeded.Error()) {
		t.Fatalf("error rows = %+v, want the error rendered", items)
	}
}

func TestBucketListNewRowCreatesABucket(t *testing.T) {
	home := t.TempDir()
	st := openStore(t, home, "r")
	v := newBucketList(home, "r")
	app := loadedBucketList(t, v)

	app = step(app, tea.KeyMsg{Type: tea.KeyEnter})
	if _, ok := app.Top().(*vkdeck.FormView); !ok {
		t.Fatalf("enter on New pushed %T, want a form", app.Top())
	}
	if got := app.Top().Title(); got != "new bucket" {
		t.Errorf("form title = %q, want new bucket", got)
	}
	for _, r := range "shift" {
		app = step(app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	step(app, tea.KeyMsg{Type: tea.KeyCtrlS})

	bs, err := st.ListBuckets(context.Background())
	if err != nil || len(bs) != 1 || bs[0].Name != "shift" {
		t.Fatalf("ListBuckets = %v err=%v, want one named shift", bs, err)
	}
	if bs[0].Kind != BucketKindUser {
		t.Errorf("kind = %q, want a user bucket", bs[0].Kind)
	}
}

func TestBucketListRefusesABlankName(t *testing.T) {
	home := t.TempDir()
	st := openStore(t, home, "r")
	v := newBucketList(home, "r")
	app := loadedBucketList(t, v)

	app = step(app, tea.KeyMsg{Type: tea.KeyEnter})
	app = step(app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	app = step(app, tea.KeyMsg{Type: tea.KeyCtrlS})

	if got := app.View(); !strings.Contains(got, "a bucket needs a name") {
		t.Fatalf("view = %q, want the blank-name complaint", got)
	}
	bs, _ := st.ListBuckets(context.Background())
	if len(bs) != 0 {
		t.Fatalf("ListBuckets = %v, want nothing created", bs)
	}
}

func TestBucketListRenameKeyEditsTheName(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	st := openStore(t, home, "r")
	if _, err := st.CreateBucket(ctx, "old", BucketKindUser, ""); err != nil {
		t.Fatal(err)
	}
	v := newBucketList(home, "r")
	app := loadedBucketList(t, v)

	app = step(app, tea.KeyMsg{Type: tea.KeyDown})
	app = step(app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if _, ok := app.Top().(*vkdeck.FormView); !ok {
		t.Fatalf("rename pushed %T, want a form", app.Top())
	}
	if got := app.Top().Title(); got != "rename old" {
		t.Errorf("form title = %q, want rename old", got)
	}
	app = step(app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'!'}})
	step(app, tea.KeyMsg{Type: tea.KeyCtrlS})

	bs, _ := st.ListBuckets(ctx)
	if len(bs) != 1 || bs[0].Name != "old!" {
		t.Fatalf("ListBuckets = %v, want the renamed bucket", bs)
	}
}

func TestBucketListRenameIgnoresTheNewRow(t *testing.T) {
	home := t.TempDir()
	v := newBucketList(home, "r")
	app := loadedBucketList(t, v)

	before := app.Top()
	app = step(app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if app.Top() != before {
		t.Fatalf("rename on the New row pushed %T", app.Top())
	}
}

func TestBucketListDeleteConfirmsThenDrops(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	st := openStore(t, home, "r")
	b, err := st.CreateBucket(ctx, "doomed", BucketKindUser, "")
	if err != nil {
		t.Fatal(err)
	}
	n, _ := st.CreateNote(ctx, "survivor", "")
	if err := st.AddMember(ctx, b.ID, n.ID, kindNote); err != nil {
		t.Fatal(err)
	}
	v := newBucketList(home, "r")
	app := loadedBucketList(t, v)

	app = step(app, tea.KeyMsg{Type: tea.KeyDown})
	app = step(app, tea.KeyMsg{Type: tea.KeyCtrlX})
	if got := app.View(); !strings.Contains(got, "delete bucket doomed?") {
		t.Fatalf("view = %q, want the confirm overlay", got)
	}
	if !strings.Contains(app.View(), "Records stay") {
		t.Errorf("confirm = %q, want it to say records stay", app.View())
	}
	app = step(app, tea.KeyMsg{Type: tea.KeyLeft})
	app = settle(app, nil)
	app = step(app, tea.KeyMsg{Type: tea.KeyEnter})
	settle(app, nil)

	bs, _ := st.ListBuckets(ctx)
	if len(bs) != 0 {
		t.Fatalf("ListBuckets = %v, want the bucket gone", bs)
	}
	notes, _ := st.ListNotes(ctx)
	if len(notes) != 1 {
		t.Fatalf("ListNotes = %v, want the note kept", notes)
	}
}

func TestBucketListDeleteCanBeDeclined(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	st := openStore(t, home, "r")
	if _, err := st.CreateBucket(ctx, "keeper", BucketKindUser, ""); err != nil {
		t.Fatal(err)
	}
	v := newBucketList(home, "r")
	app := loadedBucketList(t, v)

	app = step(app, tea.KeyMsg{Type: tea.KeyDown})
	app = step(app, tea.KeyMsg{Type: tea.KeyCtrlX})
	app = step(app, tea.KeyMsg{Type: tea.KeyEnter})
	settle(app, nil)

	bs, _ := st.ListBuckets(ctx)
	if len(bs) != 1 {
		t.Fatalf("ListBuckets = %v, want the bucket kept on the default answer", bs)
	}
}

func TestBucketListEnterOpensTheBucket(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	st := openStore(t, home, "r")
	if _, err := st.CreateBucket(ctx, "shift", BucketKindUser, ""); err != nil {
		t.Fatal(err)
	}
	v := newBucketList(home, "r")
	app := loadedBucketList(t, v)

	app = step(app, tea.KeyMsg{Type: tea.KeyDown})
	app = step(app, tea.KeyMsg{Type: tea.KeyEnter})
	app = settle(app, nil)
	if _, ok := app.Top().(*bucketView); !ok {
		t.Fatalf("enter pushed %T, want a bucketView", app.Top())
	}
	if got := app.Top().Title(); got != "bucket: shift" {
		t.Errorf("title = %q, want bucket: shift", got)
	}
}

func TestBucketListHintsAdvertiseRenameAndDelete(t *testing.T) {
	v := newBucketList(t.TempDir(), "r")
	hints := v.Hints(testScope())
	for _, want := range []string{"rename", "delete"} {
		if !hasLabel(hints, want) {
			t.Errorf("hints %v missing %q", hintLabels(hints), want)
		}
	}
}

func TestBucketListSurfacesAStoreError(t *testing.T) {
	items := bucketRows(80, nil, context.Canceled)
	if !strings.Contains(items[1].Block, "canceled") {
		t.Fatalf("rows = %+v, want the store error shown", items)
	}
}
