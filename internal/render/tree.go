package render

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"
	"github.com/codyconfer/viewkit/tree"

	"github.com/codyconfer/mino/internal/render/glyph"
	"github.com/codyconfer/mino/internal/signals"
)

func FlightTree(f layout.Frame, root string, sections []signals.Section) []tree.Row {
	th := theme.Cur()
	c := tree.DefaultConnectors()

	label := signals.CleanLine(root)
	if label == "" {
		label = "flight"
	}
	rows := make([]tree.Row, 0, len(sections)+1)
	rows = append(rows, tree.Row{Lines: []string{
		th.Title.Render(glyph.Lead(c.Trunk) + label),
	}})
	for i, s := range sections {
		last := i == len(sections)-1
		_, spad := c.Edge(last)
		rows = append(rows, tree.Branch(c, last, []string{sectionHead(th, i, s)}, ""))
		rows = append(rows, sectionLeaves(f, th, c, spad, s)...)
	}
	return rows
}

func SectionRows(f layout.Frame, sections []signals.Section) []tree.Row {
	th := theme.Cur()
	c := tree.DefaultConnectors()

	rows := make([]tree.Row, 0, len(sections))
	for i, s := range sections {
		rows = append(rows, tree.Row{Lines: []string{sectionHead(th, i, s)}})
		rows = append(rows, sectionLeaves(f, th, c, "", s)...)
	}
	return rows
}

func sectionHead(th *theme.Theme, i int, s signals.Section) string {
	title := s.Title
	if title == "" {
		title = s.Signal
	}
	title = fmt.Sprintf("%s  (%s)", signals.CleanLine(title), sectionCount(s))
	icon := th.Series[i%len(th.Series)].Render(glyph.Lead(sectionGlyph(s)))
	return icon + th.Title.Render(title) + staleChip(th, s)
}

func staleChip(th *theme.Theme, s signals.Section) string {
	return staleCue(th, s.Meta) + truncationCue(th, s.Meta)
}

func truncationCue(th *theme.Theme, meta map[string]string) string {
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

func staleCue(th *theme.Theme, meta map[string]string) string {
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

func sectionLeaves(f layout.Frame, th *theme.Theme, c tree.Connectors, spad string, s signals.Section) []tree.Row {
	switch {
	case s.Err != nil:
		errLines := strings.Split(signals.Clean(s.Err.Error()), "\n")
		body := []string{th.Cant.Render(glyph.Lead(glyph.Warn()) + errLines[0])}
		for _, l := range errLines[1:] {
			body = append(body, th.Dim.Render(l))
		}
		return []tree.Row{tree.Leaf(c, spad, true, body, "")}
	case len(s.Items) == 0:
		body := []string{th.Dim.Render(c.Empty + "nothing to show")}
		return []tree.Row{tree.Leaf(c, spad, true, body, "")}
	default:
		lf := layout.NewFrame(f.Width - c.Indent(spad))
		rows := make([]tree.Row, 0, len(s.Items))
		for j, it := range s.Items {
			rows = append(rows, tree.Leaf(c, spad, j == len(s.Items)-1, itemLines(lf, th, it), it.URL))
		}
		return rows
	}
}
