package render

import (
	"strconv"
	"strings"

	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"
	"github.com/codyconfer/viewkit/tree"

	"github.com/codyconfer/mino/internal/render/glyph"
	"github.com/codyconfer/mino/internal/signals"
)

func FlightTree(f layout.Frame, root string, sections []signals.Section) []tree.Row {
	label := signals.CleanLine(root)
	if label == "" {
		label = "flight"
	}
	if trunkIsRedundant(label, sections) {
		return SectionRows(f, sections)
	}

	th := f.Theme()
	it := newItemTheme(f.Glyphs(), th)
	c := tree.ConnectorsIn(f.Glyphs(), th)

	rows := make([]tree.Row, 0, len(sections)+1)
	rows = append(rows, tree.Row{Lines: []string{
		th.Title.Render(glyph.Lead(c.Trunk) + label),
	}})
	for i, s := range sections {
		last := i == len(sections)-1
		_, spad := c.Edge(last)
		rows = append(rows, tree.Branch(c, last, []string{sectionHead(f.Glyphs(), th, i, s)}, ""))
		rows = append(rows, sectionLeaves(f, it, c, spad, s)...)
	}
	return rows
}

func SectionRows(f layout.Frame, sections []signals.Section) []tree.Row {
	return sectionRows(f, sections, -1)
}

func sectionRows(f layout.Frame, sections []signals.Section, frame int) []tree.Row {
	th := f.Theme()
	it := newItemTheme(f.Glyphs(), th)
	c := tree.ConnectorsIn(f.Glyphs(), th)

	rows := make([]tree.Row, 0, len(sections))
	for i, s := range sections {
		rows = append(rows, tree.Row{Lines: []string{sectionHead(f.Glyphs(), th, i, s)}})
		rows = append(rows, sectionLeavesFrame(f, it, c, "", s, frame)...)
	}
	return rows
}

// trunkIsRedundant reports whether the trunk row would only repeat the single
// section head hanging beneath it.
func trunkIsRedundant(label string, sections []signals.Section) bool {
	return len(sections) == 1 && signals.CleanLine(sectionTitle(sections[0])) == label
}

func sectionTitle(s signals.Section) string {
	if s.Title == "" {
		return s.Signal
	}
	return s.Title
}

func sectionHead(g glyph.Set, th theme.Theme, i int, s signals.Section) string {
	title := signals.CleanLine(sectionTitle(s)) + "  (" + sectionCount(s) + ")"
	icon := th.Series[i%len(th.Series)].Render(glyph.Lead(sectionGlyph(g, s)))
	return icon + th.Title.Render(title) + staleChip(th, s)
}

func staleChip(th theme.Theme, s signals.Section) string {
	return staleCue(th, s.Meta) + truncationCue(th, s.Meta)
}

func truncationCue(th theme.Theme, meta map[string]string) string {
	if !isTruncated(meta) {
		return ""
	}
	if more := signals.CleanLine(meta[signals.MetaMore]); more != "" {
		return th.Cant.Render("  (truncated, +" + more + " more)")
	}
	return th.Cant.Render("  (truncated)")
}

func isTruncated(meta map[string]string) bool {
	if len(meta) == 0 {
		return false
	}
	if strings.TrimSpace(meta[signals.MetaWireTruncated]) != "" {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(meta[signals.MetaTruncated])) {
	case "", "false", "0", "no":
		return false
	}
	return true
}

func staleCue(th theme.Theme, meta map[string]string) string {
	if meta["cache"] != "stale" {
		return ""
	}
	if age := signals.CleanLine(meta["age"]); age != "" {
		return th.Dim.Render("  (stale " + age + ")")
	}
	return th.Dim.Render("  (stale)")
}

func sectionCount(s signals.Section) string {
	if s.Err != nil {
		return "!"
	}
	return strconv.Itoa(len(s.Items))
}

func sectionLeaves(f layout.Frame, t itemTheme, c tree.Connectors, spad string, s signals.Section) []tree.Row {
	return sectionLeavesFrame(f, t, c, spad, s, -1)
}

func sectionLeavesFrame(f layout.Frame, t itemTheme, c tree.Connectors, spad string, s signals.Section, frame int) []tree.Row {
	th := t.th
	switch {
	case s.Err != nil:
		var body []string
		for l := range strings.SplitSeq(signals.Clean(s.Err.Error()), "\n") {
			if len(body) == 0 {
				body = append(body, th.Cant.Render(glyph.Lead(f.Glyphs().Warn())+l))
				continue
			}
			body = append(body, th.Dim.Render(l))
		}
		return []tree.Row{tree.Leaf(c, spad, true, body, "")}
	case len(s.Items) == 0:
		body := []string{th.Dim.Italic(true).Render(glyph.Lead(strings.TrimRight(c.Empty, " ")) + "nothing to show")}
		return []tree.Row{tree.Leaf(c, spad, true, body, "")}
	default:
		lf := f.WithWidth(f.Width - c.Indent(spad))
		rows := make([]tree.Row, 0, len(s.Items))
		for j, it := range s.Items {
			row := tree.Leaf(c, spad, j == len(s.Items)-1, itemLinesInFrame(lf, t, it, frame), it.URL)
			row.Payload = ItemRef{Signal: s.Signal, Item: it, Meta: s.Meta}
			rows = append(rows, row)
		}
		return rows
	}
}
