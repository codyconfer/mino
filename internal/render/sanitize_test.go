package render

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/codyconfer/viewkit/layout"

	"github.com/codyconfer/munin/internal/render/glyph"
	"github.com/codyconfer/munin/internal/signals"
)

const (
	oscTitle    = "\x1b]0;pwned\x07"
	clearScreen = "\x1b[2J"
	fakeChip    = "\x1b[32mmerged\x1b[0m"
	homeCursor  = "\x1b[H"
)

var hostilePayloads = []string{oscTitle, clearScreen, fakeChip, homeCursor, "\x7fdel"}

func hostile(label string) string {
	var b strings.Builder
	b.WriteString(label)
	for _, p := range hostilePayloads {
		b.WriteString(p)
	}
	return b.String()
}

func pinPlain(tb testing.TB) {
	tb.Helper()
	prevProfile := lipgloss.ColorProfile()
	prevMode := glyph.CurrentMode()
	lipgloss.SetColorProfile(termenv.Ascii)
	glyph.SetMode(glyph.ModeNone)
	InstallDefaultTheme()
	tb.Cleanup(func() {
		lipgloss.SetColorProfile(prevProfile)
		glyph.SetMode(prevMode)
	})
}

func assertNoControls(t *testing.T, out string) {
	t.Helper()
	for _, bad := range []struct {
		name string
		seq  string
	}{
		{"ESC (0x1b)", "\x1b"},
		{"BEL (0x07)", "\x07"},
		{"DEL (0x7f)", "\x7f"},
	} {
		if i := strings.Index(out, bad.seq); i >= 0 {
			t.Errorf("%s survived to the terminal at byte %d\n%q", bad.name, i, out)
		}
	}
}

func TestDetailPanelStripsTerminalEscapes(t *testing.T) {
	pinPlain(t)
	f := layout.NewFrame(80)

	cases := []struct {
		name string
		ref  func(ItemRef) ItemRef
		det  func(*signals.ItemDetail) *signals.ItemDetail
	}{
		{
			name: "item body",
			ref: func(r ItemRef) ItemRef {
				r.Item.Body = "## " + hostile("summary") + "\n\nsecond line " + oscTitle
				return r
			},
			det: func(*signals.ItemDetail) *signals.ItemDetail { return nil },
		},
		{
			name: "item meta and titles",
			ref: func(r ItemRef) ItemRef {
				r.Item.Title = hostile("title")
				r.Item.Subtitle = hostile("subtitle")
				r.Item.Kind = hostile("kind")
				r.Item.Meta["labels"] = hostile("labels")
				r.Meta = map[string]string{"cache": "stale", "age": hostile("5m")}
				return r
			},
			det: func(*signals.ItemDetail) *signals.ItemDetail { return nil },
		},
		{
			name: "detail body",
			det: func(d *signals.ItemDetail) *signals.ItemDetail {
				d.Body = "## " + hostile("summary") + "\n\n- bullet " + clearScreen + "\n\n```\ncode " + oscTitle + "\n```\n"
				return d
			},
		},
		{
			name: "detail rows",
			det: func(d *signals.ItemDetail) *signals.ItemDetail {
				d.Rows = [][2]string{{hostile("label"), hostile("value")}}
				return d
			},
		},
		{
			name: "detail chips and title",
			det: func(d *signals.ItemDetail) *signals.ItemDetail {
				d.Title = hostile("detail title")
				d.Kind = hostile("kind")
				d.Chips[0].Label = fakeChip
				return d
			},
		},
		{
			name: "section rows",
			det: func(d *signals.ItemDetail) *signals.ItemDetail {
				d.Sections = []signals.DetailSection{{Title: "checks", Rows: [][2]string{{hostile("lint"), hostile("failure")}}}}
				return d
			},
		},
		{
			name: "section lines",
			det: func(d *signals.ItemDetail) *signals.ItemDetail {
				d.Sections = []signals.DetailSection{{Title: "files", Lines: []string{hostile("one.go"), clearScreen + oscTitle}}}
				return d
			},
		},
		{
			name: "section body and title",
			det: func(d *signals.ItemDetail) *signals.ItemDetail {
				d.Sections = []signals.DetailSection{{Title: hostile("comments"), Body: "### @attacker\n\n" + hostile("please clamp")}}
				return d
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ref := detailRef()
			if c.ref != nil {
				ref = c.ref(ref)
			}
			var d *signals.ItemDetail
			if c.det != nil {
				d = c.det(enrichedDetail())
			}
			assertNoControls(t, DetailPanel(f, ref, d))
		})
	}
}

func TestDetailPanelKeepsHostileTextVisible(t *testing.T) {
	pinPlain(t)
	d := enrichedDetail()
	d.Body = "## summary" + oscTitle + "\n\nline one\nline two " + clearScreen
	d.Sections = []signals.DetailSection{
		{Title: "files", Lines: []string{"one.go" + oscTitle}},
		{Title: "checks", Rows: [][2]string{{"lint", "failure" + clearScreen}}},
	}
	out := DetailPanel(layout.NewFrame(80), detailRef(), d)

	for _, want := range []string{"summary", "pwned", "line one", "line two", "one.go", "failure"} {
		if !strings.Contains(out, want) {
			t.Errorf("sanitising dropped visible text %q\n%s", want, out)
		}
	}
	if !strings.Contains(out, "line one") || !strings.Contains(out, "line two") {
		t.Error("body newlines must survive sanitising")
	}
}

func TestItemRowsStripTerminalEscapes(t *testing.T) {
	pinPlain(t)
	it := signals.Item{
		Kind:      hostile("pr"),
		Title:     hostile("title"),
		Subtitle:  hostile("subtitle"),
		URL:       "https://example.invalid/1" + oscTitle,
		Timestamp: time.Now().Add(-time.Hour),
		Meta: map[string]string{
			"author":            hostile("cody"),
			"last_comment_by":   hostile("alice"),
			"last_comment_team": "true",
		},
	}
	rows := ItemRows(layout.NewFrame(80), []signals.Item{it})
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	assertNoControls(t, rows[0].Block)

	assertNoControls(t, Panels(layout.NewFrame(80), hostile("root"), []signals.Section{
		{Signal: hostile("github"), Title: hostile("Open PRs"), Items: []signals.Item{it}, Meta: map[string]string{"cache": "stale", "age": hostile("5m")}},
	}))
}

func benignDetailFixture() (ItemRef, *signals.ItemDetail) {
	ref := ItemRef{
		Signal: "github",
		Item: signals.Item{
			Kind:      "pr",
			Title:     "Clamp the poller backoff · fix #412",
			Subtitle:  "acme/tools · In Review",
			Body:      "ignored when a detail is present",
			URL: "https://github.com/acme/tools/pull/412",
			Meta: map[string]string{
				"author": "cody", "state": "open", "labels": "bug, area/signals",
				"assignees": "alice, mira", "last_comment_by": "sven", "draft": "true",
			},
		},
		Meta: map[string]string{"cache": "stale", "age": "12m"},
	}
	d := &signals.ItemDetail{
		Kind:  "pr",
		Title: "Clamp the poller backoff · fix #412",
		URL:   "https://github.com/acme/tools/pull/412",
		Rows: [][2]string{
			{"repo", "acme/tools #412"},
			{"diff", "+42 −7 across 3 files"},
			{"emoji", "shipped 🚀 いいね"},
		},
		Body: "## Summary\n\nclamp `backoff` to **[1s, 5m]**\n\n- reuse the idle timer\n- drop the redundant `fsync`\n\n```\ngo test ./...\tone\ttab\n```\n\nSee [the plan](https://example.invalid/plan).\n",
		Sections: []signals.DetailSection{
			{Title: "checks", Rows: [][2]string{{"lint", "success in 41s"}, {"unit\tcolumn", "failure in 2m18s"}}},
			{Title: "files", Lines: []string{"internal/signals/cache.go\t+88 −12", "internal/app/flight.go +31 −4"}},
			{Title: "comments", Body: "### @alice · 2h ago\n\nplease clamp the **upper** bound too\n"},
		},
	}
	return ref, d
}

const benignGolden = "testdata/detail_benign.golden"

func TestDetailPanelBenignOutputIsByteIdentical(t *testing.T) {
	pinPlain(t)
	ref, d := benignDetailFixture()
	got := DetailPanel(layout.NewFrame(80), ref, d)

	want, err := os.ReadFile(benignGolden)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got != string(want) {
		t.Errorf("benign detail rendering changed\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestDetailPanelDoesNotMutateItsInput(t *testing.T) {
	pinPlain(t)
	ref := detailRef()
	ref.Item.Body = "body " + oscTitle
	ref.Item.Meta["labels"] = "labels " + clearScreen
	d := enrichedDetail()
	d.Body = "detail " + oscTitle
	d.Rows = [][2]string{{"repo", "acme/tools " + clearScreen}}
	d.Sections = []signals.DetailSection{{Title: "files", Lines: []string{"one.go " + oscTitle}}}

	DetailPanel(layout.NewFrame(80), ref, d)

	if !strings.Contains(ref.Item.Body, oscTitle) || !strings.Contains(ref.Item.Meta["labels"], clearScreen) {
		t.Error("DetailPanel mutated the caller's item")
	}
	if !strings.Contains(d.Body, oscTitle) || !strings.Contains(d.Rows[0][1], clearScreen) ||
		!strings.Contains(d.Sections[0].Lines[0], oscTitle) {
		t.Error("DetailPanel mutated the caller's detail")
	}
}
