package render

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"
	"github.com/codyconfer/viewkit/tree"

	"github.com/codyconfer/munin/internal/render/glyph"
	"github.com/codyconfer/munin/internal/signals"
)

func FlightTree(f layout.Frame, root string, sections []signals.Section) []tree.Row {
	th := theme.Cur()
	c := tree.DefaultConnectors()

	label := signals.Clean(root)
	if label == "" {
		label = "flight"
	}
	rows := make([]tree.Row, 0, len(sections)+1)
	rows = append(rows, tree.Row{Lines: []string{
		th.Title.Render(glyph.Lead(c.Trunk) + label + fmt.Sprintf("  (%d)", len(sections))),
	}})

	for i, s := range sections {
		last := i == len(sections)-1
		_, spad := c.Edge(last)

		title := s.Title
		if title == "" {
			title = s.Signal
		}
		title = fmt.Sprintf("%s  (%s)", signals.Clean(title), sectionCount(s))
		icon := th.Series[i%len(th.Series)].Render(glyph.Lead(sectionGlyph(s)))

		rows = append(rows, tree.Branch(c, last, []string{icon + th.Title.Render(title) + staleChip(th, s)}, ""))
		rows = append(rows, sectionLeaves(f, th, c, spad, s)...)
	}
	return rows
}

func staleChip(th *theme.Theme, s signals.Section) string {
	if s.Meta["cache"] != "stale" {
		return ""
	}
	if age := signals.Clean(s.Meta["age"]); age != "" {
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
