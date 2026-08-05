package views

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/keys"

	"github.com/codyconfer/mino/internal/app/pane"
	"github.com/codyconfer/mino/internal/audit"
	"github.com/codyconfer/mino/internal/deck"
	"github.com/codyconfer/mino/internal/plugin/ntr"
	"github.com/codyconfer/mino/internal/render"
	"github.com/codyconfer/mino/internal/signals"
)

func bucketSections() []signals.Section {
	return []signals.Section{{
		Signal: "github",
		Title:  "github",
		Items: []signals.Item{
			{Kind: "pull", Title: "fix the thing", URL: "https://github.com/o/r/pull/7", Subtitle: "o/r · open"},
		},
	}}
}

func TestBucketsEnabledNeedsAHome(t *testing.T) {
	kit := testKit(t)
	if !kit.bucketsEnabled() {
		t.Fatal("bucketsEnabled = false with a home set and ntr linked in")
	}
	kit.d.App.Cfg.Home = ""
	if kit.bucketsEnabled() {
		t.Fatal("bucketsEnabled = true with no home")
	}
}

func TestFlightResultsGetsTheFileKey(t *testing.T) {
	kit := testKit(t)
	kit.d.FetchFlightAudited = func(string) []signals.Section { return bucketSections() }

	v := kit.FlightResults("default")
	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	app = settle(app, v.Init())

	if !hasHintLabel(v.Hints(app.UI()), "file") {
		t.Fatalf("hints = %v, want a file hint", hintLabelsOf(v.Hints(app.UI())))
	}
	app = step(app, tea.KeyMsg{Type: tea.KeyDown})
	app = stepSettle(app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if got := app.Top().Title(); got != "file into" {
		t.Fatalf("f pushed %T titled %q, want the bucket picker", app.Top(), got)
	}
}

func TestFlightResultsStillExposesItsPaneSnapshot(t *testing.T) {
	kit := testKit(t)
	kit.d.FetchFlightAudited = func(string) []signals.Section { return bucketSections() }

	v := kit.FlightResults("default")
	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	app = settle(app, v.Init())

	src, ok := v.(interface {
		PaneSnapshot() (pane.Snapshot, bool)
	})
	if !ok {
		t.Fatal("wrapping for the file key dropped PaneSnapshot")
	}
	snap, ok := src.PaneSnapshot()
	if !ok {
		t.Fatal("PaneSnapshot reported nothing after a load")
	}
	if snap.Kind != pane.KindSections || snap.Origin != "flight:default" {
		t.Fatalf("snapshot = %+v, want the flight's sections", snap)
	}
	if !strings.Contains(app.View(), "fix the thing") {
		t.Fatalf("view = %q, want the result row still rendered", app.View())
	}
}

func TestFlightResultsWithoutHomeHasNoFileKey(t *testing.T) {
	kit := testKit(t)
	kit.d.App.Cfg.Home = ""
	kit.d.FetchFlightAudited = func(string) []signals.Section { return bucketSections() }

	v := kit.FlightResults("default")
	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	app = settle(app, v.Init())

	if hasHintLabel(v.Hints(app.UI()), "file") {
		t.Fatal("hints advertised file with no home configured")
	}
}

func TestDetailViewFileKeyPushesThePicker(t *testing.T) {
	kit := testKit(t)
	ref := render.ItemRef{Signal: "github", Item: bucketSections()[0].Items[0]}
	v := kit.Detail(ref)
	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	app = settle(app, v.Init())

	if !hasHintLabel(v.Hints(app.UI()), "file") {
		t.Fatalf("hints = %v, want a file hint", hintLabelsOf(v.Hints(app.UI())))
	}
	app = stepSettle(app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if got := app.Top().Title(); got != "file into" {
		t.Fatalf("f pushed %T titled %q, want the bucket picker", app.Top(), got)
	}
}

func TestDetailViewWithoutAURLHasNoFileKey(t *testing.T) {
	kit := testKit(t)
	ref := render.ItemRef{Signal: "ntr", Item: signals.Item{Kind: "note", Title: "no url"}}
	v := kit.Detail(ref)
	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	app = settle(app, v.Init())

	if hasHintLabel(v.Hints(app.UI()), "file") {
		t.Fatal("hints advertised file for an item with no URL")
	}
	before := app.Top()
	app = step(app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if app.Top() != before {
		t.Fatalf("f pushed %T for an unanchorable item", app.Top())
	}
}

func TestHistoryRunKeepsDeleteAndGainsFile(t *testing.T) {
	kit := testKit(t)
	row := audit.AuditRow{ID: 184, Kind: "flight", Name: "mino-ci", ItemCount: 1}
	v := kit.historyRun(row)
	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	app = settle(app, v.Init())

	labels := hintLabelsOf(v.Hints(app.UI()))
	for _, want := range []string{"delete", "file"} {
		if !hasHintLabel(v.Hints(app.UI()), want) {
			t.Fatalf("hints = %v, want %q", labels, want)
		}
	}

	app = stepSettle(app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if got := app.Top().Title(); got != "file into" {
		t.Fatalf("f pushed %T titled %q, want the bucket picker", app.Top(), got)
	}
}

func TestHistoryRunFileAnchorsOnTheRun(t *testing.T) {
	kit := testKit(t)
	home := kit.d.App.Cfg.Home
	row := audit.AuditRow{ID: 184, Kind: "flight", Name: "mino-ci"}
	v := kit.historyRun(row)
	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	app = settle(app, v.Init())

	app = stepSettle(app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if got := app.View(); !strings.Contains(got, "This run") {
		t.Fatalf("picker = %q, want a This run row", got)
	}

	app = stepSettle(app, tea.KeyMsg{Type: tea.KeyDown})
	app = stepSettle(app, tea.KeyMsg{Type: tea.KeyEnter})
	stepSettle(app, tea.KeyMsg{Type: tea.KeyEnter})

	st, err := ntr.Open(context.Background(), home, kit.d.App.Role())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	b, ok, err := st.BucketByAnchor(context.Background(), ntr.BucketKindRun, ntr.RunAnchor(184))
	if err != nil || !ok {
		t.Fatalf("BucketByAnchor = %+v ok=%v err=%v, want a run bucket ensured", b, ok, err)
	}
	if !strings.Contains(b.Name, "mino-ci") {
		t.Errorf("bucket name = %q, want the flight name", b.Name)
	}
}

func TestHistoryRunDeleteStillConfirms(t *testing.T) {
	kit := testKit(t)
	row := audit.AuditRow{ID: 5, Kind: "query", Name: "q1"}
	v := kit.historyRun(row)
	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	app = settle(app, v.Init())

	app = step(app, tea.KeyMsg{Type: tea.KeyCtrlX})
	if got := app.View(); !strings.Contains(got, "delete run #5?") {
		t.Fatalf("view = %q, want the delete confirm intact", got)
	}
}

func TestNTRBucketsOpensTheIndex(t *testing.T) {
	kit := testKit(t)
	v := kit.NTRBuckets()
	if got := v.Title(); got != "buckets" {
		t.Fatalf("title = %q, want buckets", got)
	}
}

func stepSettle(a *vkdeck.Model, msg tea.Msg) *vkdeck.Model {
	m, cmd := a.Update(msg)
	return settle(m.(*vkdeck.Model), cmd)
}

func hintLabelsOf(hints []keys.Hint) []string {
	out := make([]string, 0, len(hints))
	for _, h := range hints {
		out = append(out, h.Label)
	}
	return out
}

func hasHintLabel(hints []keys.Hint, label string) bool {
	for _, h := range hints {
		if h.Label == label {
			return true
		}
	}
	return false
}

func TestStampFiledOnlyMarksHitItems(t *testing.T) {
	in := []signals.Section{{
		Signal: "github",
		Title:  "github",
		Items: []signals.Item{
			{Kind: "pull", Title: "filed", URL: "https://x/1", Meta: map[string]string{"author": "a"}},
			{Kind: "pull", Title: "bare", URL: "https://x/2"},
			{Kind: "note", Title: "no url"},
		},
	}}
	out := stampFiled(in, map[string]int{"https://x/1": 3})

	if got := out[0].Items[0].Meta[signals.MetaFiled]; got != "3" {
		t.Fatalf("hit item filed = %q, want 3", got)
	}
	if got := out[0].Items[0].Meta["author"]; got != "a" {
		t.Errorf("hit item lost its other meta: %v", out[0].Items[0].Meta)
	}
	if _, ok := out[0].Items[1].Meta[signals.MetaFiled]; ok {
		t.Error("an unfiled item was stamped")
	}
	if _, ok := out[0].Items[2].Meta[signals.MetaFiled]; ok {
		t.Error("an item with no URL was stamped")
	}
}

func TestStampFiledDoesNotMutateItsInput(t *testing.T) {
	shared := map[string]string{"author": "a"}
	in := []signals.Section{{
		Signal: "github",
		Items:  []signals.Item{{Kind: "pull", URL: "https://x/1", Meta: shared}},
	}}
	out := stampFiled(in, map[string]int{"https://x/1": 2})

	if _, ok := shared[signals.MetaFiled]; ok {
		t.Fatal("stampFiled wrote into the caller's Meta map")
	}
	if _, ok := in[0].Items[0].Meta[signals.MetaFiled]; ok {
		t.Fatal("stampFiled mutated the input item")
	}
	if got := out[0].Items[0].Meta[signals.MetaFiled]; got != "2" {
		t.Fatalf("output filed = %q, want 2", got)
	}
	if &in[0].Items[0] == &out[0].Items[0] {
		t.Fatal("stampFiled reused the input item backing array")
	}
}

func TestWithFiledCountsNoOpsWithoutAHome(t *testing.T) {
	kit := testKit(t)
	kit.d.App.Cfg.Home = ""
	in := bucketSections()
	out := kit.withFiledCounts(in)
	if _, ok := out[0].Items[0].Meta[signals.MetaFiled]; ok {
		t.Fatal("stamped a cue with no home configured")
	}
}

func TestWithFiledCountsStampsFromTheStore(t *testing.T) {
	kit := testKit(t)
	ctx := context.Background()
	home := kit.d.App.Cfg.Home
	st, err := ntr.Open(ctx, home, kit.d.App.Role())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	url := bucketSections()[0].Items[0].URL
	b, _ := st.EnsureAnchorBucket(ctx, ntr.BucketKindItem, url, "PR #7")
	n, _ := st.CreateNote(ctx, "about it", "")
	if err := st.AddMember(ctx, b.ID, n.ID, "note"); err != nil {
		t.Fatal(err)
	}

	out := kit.withFiledCounts(bucketSections())
	if got := out[0].Items[0].Meta[signals.MetaFiled]; got != "1" {
		t.Fatalf("filed = %q, want 1", got)
	}
}

func TestWithFiledCountsSkipsWhenNothingIsFiled(t *testing.T) {
	kit := testKit(t)
	in := bucketSections()
	out := kit.withFiledCounts(in)
	if _, ok := out[0].Items[0].Meta[signals.MetaFiled]; ok {
		t.Fatal("stamped a cue with an empty store")
	}
}

func TestDetailContextShowsTheFiledCue(t *testing.T) {
	kit := testKit(t)
	it := bucketSections()[0].Items[0]
	it.Meta = map[string]string{signals.MetaFiled: "2"}
	v := kit.Detail(render.ItemRef{Signal: "github", Item: it})
	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 120, Height: 40})

	var found string
	for _, h := range v.Context(app.UI()) {
		if h.Key == "notes" {
			found = h.Label
		}
	}
	if found != "2" {
		t.Fatalf("context cues = %+v, want notes 2", v.Context(app.UI()))
	}
}

func TestDetailContextHidesAZeroFiledCue(t *testing.T) {
	kit := testKit(t)
	it := bucketSections()[0].Items[0]
	it.Meta = map[string]string{signals.MetaFiled: "0"}
	v := kit.Detail(render.ItemRef{Signal: "github", Item: it})
	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 120, Height: 40})

	for _, h := range v.Context(app.UI()) {
		if h.Key == "notes" {
			t.Fatalf("context showed a zero cue: %+v", h)
		}
	}
}

func TestHistoryRunContextShowsTheRunCue(t *testing.T) {
	kit := testKit(t)
	ctx := context.Background()
	st, err := ntr.Open(ctx, kit.d.App.Cfg.Home, kit.d.App.Role())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	b, _ := st.EnsureAnchorBucket(ctx, ntr.BucketKindRun, ntr.RunAnchor(184), "run 184")
	n, _ := st.CreateNote(ctx, "about the run", "")
	if err := st.AddMember(ctx, b.ID, n.ID, "note"); err != nil {
		t.Fatal(err)
	}

	v := kit.historyRun(audit.AuditRow{ID: 184, Kind: "flight", Name: "mino-ci"})
	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 120, Height: 40})

	var found string
	for _, h := range v.Context(app.UI()) {
		if h.Key == "notes" {
			found = h.Label
		}
	}
	if found != "1" {
		t.Fatalf("context cues = %+v, want notes 1", v.Context(app.UI()))
	}
}

func TestBucketsHotkeyOpensTheIndex(t *testing.T) {
	kit := testKit(t)
	kit.d.App.Cfg.Keybinds = map[string]string{"alt+b": "ntr.buckets"}

	app := newTestApp(kit.Home(), deck.WithKeyHook(kit.KeyHook()))
	app = step(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	app = stepSettle(app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}, Alt: true})

	if got := app.Top().Title(); got != "buckets" {
		t.Fatalf("alt+b pushed %T titled %q, want the buckets index", app.Top(), got)
	}
}

func TestBucketsHotkeyHintIsListed(t *testing.T) {
	kit := testKit(t)
	kit.d.App.Cfg.Keybinds = map[string]string{"alt+b": "ntr.buckets"}
	if !hasHintLabel(kit.hotkeyHints(), "buckets") {
		t.Fatalf("hotkey hints = %v, want a buckets entry", hintLabelsOf(kit.hotkeyHints()))
	}
}
