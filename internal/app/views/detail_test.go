package views

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/codyconfer/viewkit/glyph"
	"github.com/codyconfer/viewkit/ui"

	"github.com/codyconfer/mino/internal/render"
	"github.com/codyconfer/mino/internal/signals"
)

func detailTestRef() render.ItemRef {
	return render.ItemRef{
		Signal: "github",
		Item: signals.Item{
			Kind:     "pr",
			Title:    "Fix rate-limit backoff",
			Subtitle: "acme/tools · In Review",
			Body:     "local body text",
			URL:      "https://github.com/acme/tools/pull/412",
			Meta:     map[string]string{"author": "cody", "state": "open"},
		},
	}
}

func enrichedTestDetail() *signals.ItemDetail {
	return &signals.ItemDetail{
		Kind:  "pr",
		Title: "Fix rate-limit backoff",
		Chips: []signals.Chip{{Label: "open", Sev: glyph.SeverityNeutral}},
		Rows:  [][2]string{{"repo", "acme/tools #412"}},
		Body:  "enriched body text",
		Sections: []signals.DetailSection{
			{Title: "checks", Rows: [][2]string{{"lint", "failure"}}},
		},
	}
}

func detailView(t *testing.T, fetch func(string, signals.Item) (*signals.ItemDetail, error)) *DetailView {
	t.Helper()
	kit := testKit(t)
	kit.d.FetchDetail = fetch
	v, ok := kit.Detail(detailTestRef()).(*DetailView)
	if !ok {
		t.Fatal("Kit.Detail did not return a *DetailView")
	}
	return v
}

func TestDetailViewTitleAndContext(t *testing.T) {
	v := detailView(t, nil)
	if got := v.Title(); got != "pr #412" {
		t.Errorf("Title = %q, want %q", got, "pr #412")
	}
	cues := map[string]string{}
	for _, c := range v.Context(ui.Default()) {
		cues[c.Key] = c.Label
	}
	if cues["repo"] != "acme/tools" {
		t.Errorf("context = %v, want repo=acme/tools", cues)
	}
}

func TestDetailViewRendersLocalBeforeEnrichment(t *testing.T) {
	glyph.SetMode(glyph.ModeNone)
	v := detailView(t, func(string, signals.Item) (*signals.ItemDetail, error) {
		return enrichedTestDetail(), nil
	})
	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})

	out := ansi.Strip(app.View())
	if !strings.Contains(out, "local body text") {
		t.Errorf("pre-enrich frame should show local data\n%s", out)
	}
	if strings.Contains(out, "enriched body text") {
		t.Error("enriched content appeared before the fetch resolved")
	}
}

func TestDetailViewShowsLoadingCueThenEnriches(t *testing.T) {
	glyph.SetMode(glyph.ModeNone)
	v := detailView(t, func(string, signals.Item) (*signals.ItemDetail, error) {
		return enrichedTestDetail(), nil
	})
	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})

	cmd := v.Init()
	if cmd == nil {
		t.Fatal("Init should start the enrich fetch")
	}
	if !strings.Contains(ansi.Strip(app.View()), "loading") {
		t.Errorf("want a loading cue while enriching\n%s", ansi.Strip(app.View()))
	}

	app = step(app, cmd())
	out := ansi.Strip(app.View())
	for _, want := range []string{"enriched body text", "acme/tools #412", "checks", "lint"} {
		if !strings.Contains(out, want) {
			t.Errorf("enriched frame missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "loading") {
		t.Error("loading cue should clear once the detail lands")
	}
}

func TestDetailViewTicksWhileWorkflowIsInProgress(t *testing.T) {
	detail := enrichedTestDetail()
	detail.Sections = append(detail.Sections, signals.DetailSection{
		Title: "workflow · CI",
		Rows:  [][2]string{{"test", "in progress"}, {"  ↳ go test", "in progress"}},
		Meta:  map[string]string{"in_progress": "true"},
	})
	v := detailView(t, nil)
	if cmd := v.Update(nil, detailLoadedMsg{detail: detail}); cmd == nil {
		t.Fatal("in-progress workflow did not start the animation tick")
	}
	before := v.frame
	if cmd := v.Update(nil, detailAnimationMsg{}); cmd == nil {
		t.Fatal("in-progress workflow did not continue the animation tick")
	}
	if v.frame != before+1 {
		t.Errorf("frame = %d, want %d", v.frame, before+1)
	}
	detail.Sections[len(detail.Sections)-1].Meta["in_progress"] = "false"
	if cmd := v.Update(nil, detailAnimationMsg{}); cmd != nil {
		t.Error("completed workflow kept the animation tick alive")
	}
}

func TestDetailViewPollsAndStopsWhenWorkflowSettles(t *testing.T) {
	ref := detailTestRef()
	ref.Item.Kind = "workflow"
	ref.Item.Meta = map[string]string{"status": "in_progress", "run_id": "42"}
	active := &signals.ItemDetail{
		Kind: "workflow",
		Sections: []signals.DetailSection{{
			Meta: map[string]string{"run_id": "42", "in_progress": "true"},
		}},
	}
	settled := &signals.ItemDetail{
		Kind: "workflow",
		Sections: []signals.DetailSection{{
			Meta: map[string]string{"run_id": "42"},
			Rows: [][2]string{{"test", "success"}},
		}},
	}
	var calls int
	v := &DetailView{
		ref: ref, detail: active, pollInterval: time.Minute, pollGeneration: 3,
		fetch: func(string, signals.Item) (*signals.ItemDetail, error) {
			calls++
			return settled, nil
		},
	}
	cmd := v.Update(nil, detailPollMsg{generation: 3})
	if cmd == nil {
		t.Fatal("in-progress detail did not fetch on a poll")
	}
	msg, ok := cmd().(detailLoadedMsg)
	if !ok || calls != 1 {
		t.Fatalf("poll result = %T, calls = %d; want detailLoadedMsg, 1", msg, calls)
	}
	v.Update(nil, msg)
	if render.ItemInProgress(v.ref.Item) {
		t.Fatal("settled workflow kept the stale result spinner")
	}
	if cmd := v.Update(nil, detailPollMsg{generation: v.pollGeneration}); cmd != nil {
		t.Fatal("settled detail continued polling")
	}
}

func TestDetailViewKeepsLocalFrameOnFetchError(t *testing.T) {
	glyph.SetMode(glyph.ModeNone)
	v := detailView(t, func(string, signals.Item) (*signals.ItemDetail, error) {
		return nil, errors.New("network down")
	})
	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	app = step(app, v.Init()())

	out := ansi.Strip(app.View())
	if !strings.Contains(out, "network down") {
		t.Errorf("want the error surfaced\n%s", out)
	}
	if !strings.Contains(out, "local body text") {
		t.Errorf("a failed enrich must not destroy the local frame\n%s", out)
	}
	if !strings.Contains(out, "unavailable") {
		t.Errorf("want an unavailable context cue\n%s", out)
	}
}

func TestDetailViewOpenAndCancel(t *testing.T) {
	glyph.SetMode(glyph.ModeNone)
	v := detailView(t, nil)

	var opened string
	v.open = func(url string) error {
		opened = url
		return nil
	}

	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})

	_, cmd := app.Update(detailKey("o"))
	if cmd == nil {
		t.Fatal("o should produce an open command")
	}
	cmd()
	if opened != detailTestRef().Item.URL {
		t.Errorf("opened %q, want the item URL", opened)
	}

	if _, cmd := app.Update(detailKey("esc")); cmd == nil {
		t.Error("esc should pop the view")
	}
}

func TestDetailViewHintsMatchItsBindings(t *testing.T) {
	v := detailView(t, nil)
	labels := map[string]bool{}
	for _, h := range v.Hints(ui.Default()) {
		labels[h.Label] = true
	}
	for _, want := range []string{"scroll", "page", "open"} {
		if !labels[want] {
			t.Errorf("hints %v missing %q", v.Hints(ui.Default()), want)
		}
	}

	noURL := detailTestRef()
	noURL.Item.URL = ""
	bare := &DetailView{ref: noURL}
	for _, h := range bare.Hints(ui.Default()) {
		if h.Label == "open" {
			t.Error("an item without a URL should not advertise open")
		}
	}
}

func TestDetailViewScrolls(t *testing.T) {
	glyph.SetMode(glyph.ModeNone)
	ref := detailTestRef()
	ref.Item.Body = strings.Repeat("a long paragraph of body text\n", 80)
	v := &DetailView{ref: ref}

	app := newTestApp(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 20})
	_ = app.View()

	if v.scroll.Offset != 0 {
		t.Fatalf("initial offset = %d", v.scroll.Offset)
	}
	app = step(app, detailKey("down"))
	if v.scroll.Offset != 1 {
		t.Errorf("offset after down = %d, want 1", v.scroll.Offset)
	}
	app = step(app, tea.KeyMsg{Type: tea.KeyPgDown})
	if v.scroll.Offset <= 1 {
		t.Errorf("offset after pgdn = %d, want a page jump", v.scroll.Offset)
	}
	before := v.scroll.Offset
	for range 500 {
		app = step(app, detailKey("down"))
	}
	if v.scroll.Offset <= before {
		t.Error("offset should advance toward the end")
	}
	if v.scroll.Offset >= v.scroll.Total() {
		t.Errorf("offset %d should stay clamped below total %d", v.scroll.Offset, v.scroll.Total())
	}
}

func detailKey(s string) tea.KeyMsg {
	switch s {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func TestDetailViewAnimatesWhenOnlyTheItemIsInProgress(t *testing.T) {
	v := detailView(t, nil)
	v.ref.Item = signals.Item{
		Kind:  "workflow",
		Title: "CI #42",
		URL:   "https://github.com/acme/tools/actions/runs/42",
		Meta:  map[string]string{"status": "in_progress", "state": "in progress"},
	}
	if cmd := v.Init(); cmd == nil {
		t.Fatal("in-progress workflow item did not start the animation tick")
	}
	if got := v.animFrame(); got != 0 {
		t.Fatalf("animFrame = %d, want 0 once animating", got)
	}
	if cmd := v.Update(nil, detailAnimationMsg{}); cmd == nil {
		t.Fatal("in-progress workflow item did not continue the animation tick")
	}
	if v.frame != 1 {
		t.Errorf("frame = %d, want 1", v.frame)
	}
}

func TestDetailViewSettledItemRendersNoSpinnerFrame(t *testing.T) {
	v := detailView(t, nil)
	if cmd := v.Init(); cmd != nil {
		t.Fatal("settled detail should not start an animation tick")
	}
	if got := v.animFrame(); got != -1 {
		t.Fatalf("animFrame = %d, want -1 while nothing is in progress", got)
	}
}
