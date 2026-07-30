package render

import (
	"bytes"
	"flag"
	"os"
	"slices"
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
	c1CSI       = "\u009b"
	c1NEL       = "\u0085"
	forgedRow   = "     FAKE ROW injected  (999)"
)

var hostilePayloads = []string{oscTitle, clearScreen, fakeChip, homeCursor, "\x7fdel", "\n" + forgedRow}

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
	if n := strings.Count(rows[0].Block, "\n"); n != 1 {
		t.Errorf("row block has %d line breaks, want 1 (head plus URL)\n%q", n, rows[0].Block)
	}
	if strings.Contains(rows[0].Block, "\t") {
		t.Errorf("row block kept a tab\n%q", rows[0].Block)
	}

	tree := Panels(layout.NewFrame(80), hostile("root"), []signals.Section{
		{Signal: hostile("github"), Title: hostile("Open PRs"), Items: []signals.Item{it}, Meta: map[string]string{"cache": "stale", "age": hostile("5m")}},
	})
	assertNoControls(t, tree)
	if n := strings.Count(tree, "\n"); n != 3 {
		t.Errorf("flight tree has %d line breaks, want 3 (root, section, item head, item url)\n%s", n, tree)
	}
}

func benignDetailFixture() (ItemRef, *signals.ItemDetail) {
	ref := ItemRef{
		Signal: "github",
		Item: signals.Item{
			Kind:     "pr",
			Title:    "Clamp the poller backoff · fix #412",
			Subtitle: "acme/tools · In Review",
			Body:     "ignored when a detail is present",
			URL:      "https://github.com/acme/tools/pull/412",
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

var updateGolden = flag.Bool("update-golden", false, "rewrite the render golden files")

func TestDetailPanelBenignOutputIsByteIdentical(t *testing.T) {
	pinPlain(t)
	ref, d := benignDetailFixture()
	got := DetailPanel(layout.NewFrame(80), ref, d)

	if *updateGolden {
		if err := os.WriteFile(benignGolden, []byte(got), 0o600); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}

	want, err := os.ReadFile(benignGolden)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got != string(want) {
		t.Errorf("benign detail rendering changed\n got:\n%s\nwant:\n%s", got, want)
	}

	for _, want := range []string{"Summary", "reuse the idle timer", "go test ./...", "please clamp the"} {
		if !strings.Contains(got, want) {
			t.Errorf("benign body line %q went missing", want)
		}
	}
	if n := strings.Count(got, "\n"); n < 20 {
		t.Errorf("benign body collapsed: only %d lines", n+1)
	}
	assertNoControls(t, got)
}

func TestDetailPanelBenignSingleLineFieldsAreUntouched(t *testing.T) {
	pinPlain(t)
	ref, d := benignDetailFixture()
	before := []string{
		ref.Item.Title, ref.Item.Subtitle, ref.Item.Kind, d.Title, d.Kind,
		d.Rows[0][0], d.Rows[0][1], d.Rows[2][1], d.Sections[0].Title, d.Sections[1].Lines[1],
	}
	for _, s := range before {
		if got := signals.CleanLine(s); got != s {
			t.Errorf("CleanLine rewrote control-free text %q -> %q", s, got)
		}
	}
	if got := signals.Clean(d.Body); got != d.Body {
		t.Errorf("Clean rewrote a control-free body")
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

var boxEdges = []rune{'╭', '╰', '│', '┌', '└', '├', '┤', '─'}

func assertBoxChrome(t *testing.T, out string) {
	t.Helper()
	for i, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		if r := []rune(line)[0]; !slices.Contains(boxEdges, r) {
			t.Errorf("line %d escaped the box chrome (starts with %q)\n%s", i, r, out)
		}
	}
}

func assertSameLineCount(t *testing.T, what, benign, forged string) {
	t.Helper()
	want, got := strings.Count(benign, "\n"), strings.Count(forged, "\n")
	if got != want {
		t.Errorf("%s: a newline forged %d extra line(s)\n--- forged ---\n%s\n--- space-separated baseline ---\n%s",
			what, got-want, forged, benign)
	}
}

func TestDetailPanelSingleLineFieldsCannotForgeLines(t *testing.T) {
	pinPlain(t)
	f := layout.NewFrame(80)

	cases := []struct {
		name string
		mut  func(sep string, ref *ItemRef, d *signals.ItemDetail)
	}{
		{"chip label", func(sep string, _ *ItemRef, d *signals.ItemDetail) {
			d.Chips[0].Label = "open" + sep + "FAKE"
		}},
		{"detail kind", func(sep string, _ *ItemRef, d *signals.ItemDetail) {
			d.Kind = "pr" + sep + "FAKE"
		}},
		{"detail title", func(sep string, _ *ItemRef, d *signals.ItemDetail) {
			d.Title = "title" + sep + forgedRow
		}},
		{"item title", func(sep string, ref *ItemRef, d *signals.ItemDetail) {
			ref.Item.Title = "title" + sep + forgedRow
			d.Title = ""
		}},
		{"item subtitle", func(sep string, ref *ItemRef, _ *signals.ItemDetail) {
			ref.Item.Subtitle = "acme/tools" + sep + forgedRow
		}},
		{"item meta value", func(sep string, ref *ItemRef, d *signals.ItemDetail) {
			ref.Item.Meta["labels"] = "bug" + sep + forgedRow
			d.Rows = nil
		}},
		{"stale age", func(sep string, ref *ItemRef, _ *signals.ItemDetail) {
			ref.Meta = map[string]string{"cache": "stale", "age": "5m" + sep + forgedRow}
		}},
		{"detail rows", func(sep string, _ *ItemRef, d *signals.ItemDetail) {
			d.Rows = [][2]string{{"repo" + sep + "FAKE", "acme/tools" + sep + forgedRow}}
		}},
		{"section title", func(sep string, _ *ItemRef, d *signals.ItemDetail) {
			d.Sections[0].Title = "checks" + sep + forgedRow
		}},
		{"section rows", func(sep string, _ *ItemRef, d *signals.ItemDetail) {
			d.Sections[0].Rows = [][2]string{{"lint" + sep + "FAKE", "failure" + sep + forgedRow}}
		}},
		{"section lines", func(sep string, _ *ItemRef, d *signals.ItemDetail) {
			d.Sections[0].Rows = nil
			d.Sections[0].Lines = []string{"one.go" + sep + forgedRow}
		}},
	}

	render := func(mut func(string, *ItemRef, *signals.ItemDetail), sep string) string {
		ref, d := detailRef(), enrichedDetail()
		mut(sep, &ref, d)
		return DetailPanel(f, ref, d)
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			forged := render(c.mut, "\n")
			assertSameLineCount(t, c.name, render(c.mut, " "), forged)
			assertBoxChrome(t, forged)
		})
	}
}

func TestFlightTreeSingleLineFieldsCannotForgeRows(t *testing.T) {
	pinPlain(t)
	f := layout.NewFrame(80)

	cases := []struct {
		name string
		mut  func(sep string, root *string, s *signals.Section)
	}{
		{"root", func(sep string, root *string, _ *signals.Section) {
			*root = "flight" + sep + forgedRow
		}},
		{"section title", func(sep string, _ *string, s *signals.Section) {
			s.Title = "Open PRs" + sep + forgedRow
		}},
		{"section signal", func(sep string, _ *string, s *signals.Section) {
			s.Title = ""
			s.Signal = "github" + sep + forgedRow
		}},
		{"stale age", func(sep string, _ *string, s *signals.Section) {
			s.Meta = map[string]string{"cache": "stale", "age": "5m" + sep + forgedRow}
		}},
		{"item title", func(sep string, _ *string, s *signals.Section) {
			s.Items[0].Title = "benign" + sep + forgedRow
		}},
		{"item subtitle", func(sep string, _ *string, s *signals.Section) {
			s.Items[0].Subtitle = "acme/tools" + sep + forgedRow
		}},
		{"item url", func(sep string, _ *string, s *signals.Section) {
			s.Items[0].URL = "https://example.invalid/1" + sep + forgedRow
		}},
		{"item author", func(sep string, _ *string, s *signals.Section) {
			s.Items[0].Meta["author"] = "cody" + sep + forgedRow
		}},
	}

	render := func(mut func(string, *string, *signals.Section), sep string) (string, string) {
		root := "flight"
		s := signals.Section{
			Signal: "github",
			Title:  "Open PRs",
			Items: []signals.Item{{
				Kind: "pr", Title: "benign", Subtitle: "acme/tools",
				URL: "https://example.invalid/1", Timestamp: time.Now().Add(-time.Hour),
				Meta: map[string]string{"author": "cody"},
			}},
		}
		mut(sep, &root, &s)
		sections := []signals.Section{s}
		rows := ItemRows(f, s.Items)
		blocks := make([]string, 0, len(rows))
		for _, r := range rows {
			blocks = append(blocks, r.Block)
		}
		return Panels(f, root, sections), strings.Join(blocks, "\n")
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tree, rows := render(c.mut, "\n")
			benignTree, benignRows := render(c.mut, " ")
			assertSameLineCount(t, "Panels "+c.name, benignTree, tree)
			assertSameLineCount(t, "ItemRows "+c.name, benignRows, rows)
		})
	}
}

func TestBodyNewlinesSurviveWhileTitleNewlinesDoNot(t *testing.T) {
	pinPlain(t)
	f := layout.NewFrame(80)

	oneLine, twoLines := enrichedDetail(), enrichedDetail()
	oneLine.Body, twoLines.Body = "alpha bravo", "alpha\n\nbravo"
	if a, b := strings.Count(DetailPanel(f, detailRef(), oneLine), "\n"), strings.Count(DetailPanel(f, detailRef(), twoLines), "\n"); b <= a {
		t.Errorf("body newlines were collapsed: %d lines with a break, %d without", b, a)
	}

	spaced, broken := enrichedDetail(), enrichedDetail()
	spaced.Title, broken.Title = "alpha bravo", "alpha\nbravo"
	assertSameLineCount(t, "detail title", DetailPanel(f, detailRef(), spaced), DetailPanel(f, detailRef(), broken))
}

func TestItemLabelAndScopeAreSingleLine(t *testing.T) {
	it := signals.Item{
		Kind:     "pr\nFAKE" + oscTitle,
		Subtitle: "acme/tools\nFAKE · In Review",
		URL:      "https://example.invalid/pull/412",
	}
	label := ItemLabel(it)
	if strings.ContainsAny(label, "\n\r\t") || strings.Contains(label, "\x1b") {
		t.Errorf("ItemLabel is not a single safe line: %q", label)
	}
	if scope := ItemScope(it); strings.ContainsAny(scope, "\n\r\t") {
		t.Errorf("ItemScope is not a single line: %q", scope)
	}
}

func TestCleanMetaKeyCollisionKeepsTheBenignValue(t *testing.T) {
	pinPlain(t)
	ref := detailRef()
	ref.Item.Meta = map[string]string{
		"author":     "benign",
		"author\x1b": "ATTACKER",
		"state":      "open",
	}
	for i := 0; i < 20; i++ {
		out := plain(t, DetailPanel(layout.NewFrame(80), ref, nil))
		if !strings.Contains(out, "benign") {
			t.Fatalf("colliding meta key clobbered the benign author value\n%s", out)
		}
		if strings.Contains(out, "ATTACKER") {
			t.Fatalf("colliding meta key overwrote the benign author value\n%s", out)
		}
	}
}

func TestJSONRendererStripsC1AndDel(t *testing.T) {
	var buf bytes.Buffer
	r := &JSONRenderer{}
	sections := []signals.Section{{
		Signal: "github" + c1CSI,
		Title:  "Open PRs" + "\x7f",
		Meta:   map[string]string{"age" + c1NEL: "5m" + c1CSI},
		Items: []signals.Item{{
			Kind:     "pr" + "\x7f",
			Title:    "title" + c1CSI + "\x7f",
			Subtitle: "sub" + c1NEL,
			Body:     "body" + c1CSI + "\nsecond line",
			URL:      "https://example.invalid/1" + "\x7f",
			Meta:     map[string]string{"author": "cody" + c1CSI},
		}},
	}}
	if err := r.Render(&buf, sections); err != nil {
		t.Fatalf("Render err = %v", err)
	}
	out := buf.String()
	for _, bad := range []struct {
		name string
		seq  string
	}{
		{"DEL (0x7f)", "\x7f"},
		{"CSI (U+009B)", c1CSI},
		{"NEL (U+0085)", c1NEL},
		{"ESC (0x1b)", "\x1b"},
	} {
		if i := strings.Index(out, bad.seq); i >= 0 {
			t.Errorf("%s survived json output at byte %d\n%q", bad.name, i, out)
		}
	}
	if !strings.Contains(out, `"title"`) || !strings.Contains(out, "second line") {
		t.Errorf("json lost payload text\n%s", out)
	}
}
