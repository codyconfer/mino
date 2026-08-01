package render

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	vkglyph "github.com/codyconfer/viewkit/glyph"
	"github.com/codyconfer/viewkit/layout"

	"github.com/codyconfer/mino/internal/render/glyph"
	"github.com/codyconfer/mino/internal/signals"
)

const benchFrameWidth = 80

func pinTrueColor(tb testing.TB) {
	tb.Helper()
	prevProfile := lipgloss.ColorProfile()
	prevMode := glyph.CurrentMode()
	lipgloss.SetColorProfile(termenv.TrueColor)
	glyph.SetMode(glyph.ModeNerd)
	tb.Cleanup(func() {
		lipgloss.SetColorProfile(prevProfile)
		glyph.SetMode(prevMode)
	})
}

var benchRepos = []string{
	"acme/tools",
	"acme/signals-core",
	"acme/deck",
	"acme/infra-terraform",
	"acme/observability-agent",
}

var benchAuthors = []string{"cody", "alice", "bhargav", "mira", "sven", "quinn"}

func benchBody(i int) string {
	var b strings.Builder
	b.Grow(1400)
	fmt.Fprintf(&b, "## Summary\n\nThe %s poller re-armed its backoff timer on every tick, so a stale cache\n", benchRepos[i%len(benchRepos)])
	b.WriteString("entry could be refetched dozens of times inside a single flight. This clamps the\n")
	b.WriteString("interval and reuses the existing timer instead of allocating a fresh one.\n\n")
	b.WriteString("### Changes\n\n")
	b.WriteString("- clamp `backoff` to `[1s, 5m]` and jitter it by up to **10%**\n")
	b.WriteString("- reuse the idle timer rather than allocating one per operation\n")
	b.WriteString("- record the [cache hit ratio](https://example.invalid/dash) in the journal\n")
	b.WriteString("- drop the redundant directory `fsync` from the collection writer\n\n")
	b.WriteString("### Testing\n\n")
	fmt.Fprintf(&b, "```\ngo test ./internal/... -run Poller -count=%d\n```\n\n", i%5+1)
	b.WriteString("Verified against a recorded flight of 8 queries: the refetch count drops from\n")
	fmt.Fprintf(&b, "%d to %d and wall clock from 1.4s to 180ms. See _the plan_ for the rest.\n\n", 40+i%17, i%3+1)
	fmt.Fprintf(&b, "Closes #%d\n", 1200+i)
	return b.String()
}

func benchItem(i int, base time.Time) signals.Item {
	repo := benchRepos[i%len(benchRepos)]
	ts := base.Add(-time.Duration(i%71+1) * time.Hour)
	return signals.Item{
		Kind:      "pr",
		Title:     fmt.Sprintf("Clamp the %s poller backoff so a stale cache entry is not refetched (#%d)", repo, 400+i),
		Subtitle:  fmt.Sprintf("%s · In Review · %d approvals · %d files", repo, i%3, 3+i%9),
		Body:      benchBody(i),
		URL:       fmt.Sprintf("https://github.com/%s/pull/%d", repo, 400+i),
		Timestamp: ts,
		Meta: map[string]string{
			"author":            benchAuthors[i%len(benchAuthors)],
			"state":             "open",
			"status":            "In Review",
			"labels":            "bug, area/signals, perf",
			"assignees":         benchAuthors[(i+2)%len(benchAuthors)],
			"draft":             fmt.Sprint(i%11 == 0),
			"last_comment_by":   benchAuthors[(i+1)%len(benchAuthors)],
			"last_comment_at":   ts.Add(30 * time.Minute).Format(time.RFC3339),
			"last_comment_team": fmt.Sprint(i%2 == 0),
		},
	}
}

func benchSections(sections, itemsEach int) []signals.Section {
	base := time.Now()
	out := make([]signals.Section, 0, sections)
	for s := range sections {
		items := make([]signals.Item, 0, itemsEach)
		for i := range itemsEach {
			items = append(items, benchItem(s*itemsEach+i, base))
		}
		meta := map[string]string{"role": "reviewer"}
		if s%3 == 2 {
			meta["cache"] = "stale"
			meta["age"] = "12m"
		}
		out = append(out, signals.Section{
			Signal: []string{"github", "slack", "gcal", "gmail"}[s%4],
			Title:  fmt.Sprintf("review requests · %s", benchRepos[s%len(benchRepos)]),
			Items:  items,
			Meta:   meta,
		})
	}
	return out
}

func benchItemRef() ItemRef {
	return ItemRef{
		Signal: "github",
		Item:   benchItem(3, time.Now()),
		Meta:   map[string]string{"cache": "stale", "age": "12m", "role": "reviewer"},
	}
}

func benchDetail(i int) *signals.ItemDetail {
	return &signals.ItemDetail{
		Kind:  "pr",
		Title: benchItem(i, time.Now()).Title,
		URL:   fmt.Sprintf("https://github.com/%s/pull/%d", benchRepos[i%len(benchRepos)], 400+i),
		Chips: []signals.Chip{
			{Label: "open", Sev: vkglyph.SeverityNeutral},
			{Label: "checks failure", Sev: vkglyph.SeverityNegative},
			{Label: "2 approvals", Sev: vkglyph.SeverityPositive},
			{Label: "stale branch", Sev: vkglyph.SeverityWarning},
		},
		Rows: [][2]string{
			{"repo", benchRepos[i%len(benchRepos)] + fmt.Sprintf(" #%d", 400+i)},
			{"author", benchAuthors[i%len(benchAuthors)]},
			{"diff", "+412 −87 across 14 files"},
			{"base", "main ← perf/clamp-poller-backoff"},
			{"reviewers", strings.Join(benchAuthors[:4], ", ")},
			{"labels", "bug, area/signals, perf, needs-backport"},
		},
		Body: benchBody(i),
		Sections: []signals.DetailSection{
			{
				Title: "checks",
				Rows: [][2]string{
					{"lint", "success in 41s"},
					{"unit", "success in 2m18s"},
					{"integration", "failure in 6m02s — signals/cache TestStaleRefetch"},
					{"build (windows)", "success in 3m11s"},
				},
			},
			{
				Title: "files",
				Lines: []string{
					"internal/signals/cache/cache.go   +88 −12",
					"internal/signals/cache/poller.go  +140 −41",
					"internal/audit/audit.go           +26 −9",
					"internal/app/flight/flight.go     +31 −4",
				},
			},
			{
				Title: "comments",
				Body:  "### @alice · 2h ago\n\nplease clamp the **upper** bound too, `5m` is generous\n\n### @sven · 40m ago\n\nthe `fsync` removal needs a note in the changelog\n",
			},
		},
	}
}

var (
	benchOut    string
	benchStyles ReportStyles
)

func BenchmarkDetailPanel(b *testing.B) {
	pinTrueColor(b)
	f := layout.NewFrame(benchFrameWidth)
	ref := benchItemRef()

	b.Run("local_only", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			benchOut = DetailPanel(f, ref, nil)
		}
		if benchOut == "" {
			b.Fatal("DetailPanel rendered nothing")
		}
	})

	detail := benchDetail(3)
	b.Run("enriched_detail", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			benchOut = DetailPanel(f, ref, detail)
		}
		if benchOut == "" {
			b.Fatal("DetailPanel rendered nothing")
		}
	})
}

func BenchmarkFlightReportPanels(b *testing.B) {
	pinTrueColor(b)
	f := layout.NewFrame(benchFrameWidth)
	for _, shape := range []struct{ sections, items int }{
		{1, 25},
		{4, 25},
		{8, 50},
	} {
		sections := benchSections(shape.sections, shape.items)
		b.Run(fmt.Sprintf("sections=%d/items_each=%d", shape.sections, shape.items), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				benchOut = Panels(f, "results", sections)
			}
			if benchOut == "" {
				b.Fatal("Panels rendered nothing")
			}
		})
	}
}

func BenchmarkNewReportStyles(b *testing.B) {
	pinTrueColor(b)
	b.ReportAllocs()
	for b.Loop() {
		benchStyles = NewReportStyles(io.Discard)
	}
	if benchStyles.Title.Render("x") == "" {
		b.Fatal("NewReportStyles produced an unusable style set")
	}
}

type benchFinding struct {
	name    string
	msg     string
	snippet string
	ok      bool
	warn    bool
}

func benchFindings() []benchFinding {
	out := make([]benchFinding, 0, 56)
	for s := range 7 {
		for i := range 8 {
			f := benchFinding{
				name: fmt.Sprintf("%s/%s", benchRepos[s%len(benchRepos)], []string{"config", "roles", "flights", "queries", "formatters", "plugins", "onboarding"}[s]),
				ok:   true,
			}
			switch i % 4 {
			case 1:
				f.ok, f.warn = false, true
				f.msg = "no schedule set; this flight will only run on demand"
			case 2:
				f.ok = false
				f.msg = "query references an unknown signal"
				f.snippet = "queries:\n  - name: review-requests\n    signal: github.reviews\n    filter: team-only"
			}
			out = append(out, f)
		}
	}
	return out
}

func writeBenchReport(w io.Writer, sty ReportStyles, findings []benchFinding) {
	for s := range 7 {
		fmt.Fprintln(w, sty.Title.Render([]string{"Config", "Roles", "Flights", "Queries", "Formatters", "Plugins", "Onboarding"}[s]))
		for _, f := range findings[s*8 : s*8+8] {
			switch {
			case f.ok:
				fmt.Fprintf(w, "  %s %s\n", sty.OK.Render(glyph.Check()), sty.Name.Render(f.name))
			case f.warn:
				fmt.Fprintf(w, "  %s %s  %s\n", sty.Warn.Render(glyph.Warn()), sty.Name.Render(f.name), sty.Warn.Render(f.msg))
			default:
				fmt.Fprintf(w, "  %s %s  %s\n", sty.Err.Render(glyph.Cross()), sty.Name.Render(f.name), sty.Err.Render(f.msg))
			}
			if f.snippet != "" {
				for line := range strings.SplitSeq(f.snippet, "\n") {
					fmt.Fprintln(w, "      "+sty.Snippet.Render(line))
				}
				fmt.Fprintln(w, "      "+sty.Fix.Render("fix: point the query at github.review_requests"))
			}
		}
		fmt.Fprintln(w)
	}
}

func trueColorReportStyles() ReportStyles {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.TrueColor)
	sty := NewReportStyles(io.Discard)
	return ReportStyles{
		Title:   sty.Title.Renderer(r),
		OK:      sty.OK.Renderer(r),
		Err:     sty.Err.Renderer(r),
		Warn:    sty.Warn.Renderer(r),
		Name:    sty.Name.Renderer(r),
		Dim:     sty.Dim.Renderer(r),
		Snippet: sty.Snippet.Renderer(r),
		Fix:     sty.Fix.Renderer(r),
	}
}

func BenchmarkReportRender(b *testing.B) {
	pinTrueColor(b)
	findings := benchFindings()

	b.Run("profile=writer_default", func(b *testing.B) {
		sty := NewReportStyles(io.Discard)
		b.ReportAllocs()
		for b.Loop() {
			writeBenchReport(io.Discard, sty, findings)
		}
	})

	b.Run("profile=truecolor", func(b *testing.B) {
		sty := trueColorReportStyles()
		b.ReportAllocs()
		for b.Loop() {
			writeBenchReport(io.Discard, sty, findings)
		}
	})
}

func TestBenchFixturesRenderTrueColorSGR(t *testing.T) {
	const sgr = "\x1b[38;2;"
	pinTrueColor(t)
	if got := lipgloss.ColorProfile(); got != termenv.TrueColor {
		t.Fatalf("lipgloss.ColorProfile() = %v, want TrueColor", got)
	}

	detail := DetailPanel(layout.NewFrame(benchFrameWidth), benchItemRef(), benchDetail(3))
	if !strings.Contains(detail, sgr) {
		t.Errorf("DetailPanel output has no %q escape; benchmark would understate ANSI cost", sgr)
	}
	report := Panels(layout.NewFrame(benchFrameWidth), "results", benchSections(2, 3))
	if !strings.Contains(report, sgr) {
		t.Errorf("Panels output has no %q escape; benchmark would understate ANSI cost", sgr)
	}

	var buf strings.Builder
	writeBenchReport(&buf, trueColorReportStyles(), benchFindings())
	if !strings.Contains(buf.String(), sgr) {
		t.Errorf("truecolor report styles produced no %q escape", sgr)
	}
}
