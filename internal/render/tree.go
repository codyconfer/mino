package render

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/render/glyph"
	"github.com/codyconfer/munin/internal/signals"
)

type treeRow struct {
	Lines      []string
	Key        string
	Selectable bool
}

type conn struct {
	trunk, mid, end, vert, space, empty string
}

func connectors() conn {
	if glyph.CurrentMode() == glyph.ModeNone {
		return conn{trunk: "*", mid: "+- ", end: "`- ", vert: "|  ", space: "   ", empty: "o "}
	}
	return conn{trunk: "●", mid: "├─ ", end: "└─ ", vert: "│  ", space: "   ", empty: "∅ "}
}

func FlightTree(f layout.Frame, root string, sections []signals.Section) []treeRow {
	th := theme.Cur()
	c := connectors()

	label := signals.Clean(root)
	if label == "" {
		label = "flight"
	}
	rows := make([]treeRow, 0, len(sections)+1)
	rows = append(rows, treeRow{Lines: []string{th.Title.Render(glyph.Lead(c.trunk) + label + fmt.Sprintf("  (%d)", len(sections)))}})

	for i, s := range sections {
		last := i == len(sections)-1
		bconn, spad := c.mid, c.vert
		if last {
			bconn, spad = c.end, c.space
		}

		title := s.Title
		if title == "" {
			title = s.Signal
		}
		title = fmt.Sprintf("%s  (%s)", signals.Clean(title), sectionCount(s))
		icon := th.Series[i%len(th.Series)].Render(glyph.Lead(sectionGlyph(s)))
		rows = append(rows, treeRow{Lines: []string{th.Dim.Render(bconn) + icon + th.Title.Render(title)}})
		rows = append(rows, sectionLeaves(f, th, c, spad, s)...)
	}
	return rows
}

func sectionCount(s signals.Section) string {
	if s.Err != nil {
		return "!"
	}
	return strconv.Itoa(len(s.Items))
}

func sectionLeaves(f layout.Frame, th *theme.Theme, c conn, spad string, s signals.Section) []treeRow {
	switch {
	case s.Err != nil:
		errLines := strings.Split(signals.Clean(s.Err.Error()), "\n")
		body := []string{th.Cant.Render(glyph.Warn() + " " + errLines[0])}
		for _, l := range errLines[1:] {
			body = append(body, th.Dim.Render(l))
		}
		return []treeRow{leafRow(th, c, spad, true, body, "")}
	case len(s.Items) == 0:
		body := []string{th.Dim.Render(c.empty + "nothing to show")}
		return []treeRow{leafRow(th, c, spad, true, body, "")}
	default:
		indent := lipgloss.Width(spad) + lipgloss.Width(c.mid)
		lf := layout.NewFrame(f.Width - indent)
		rows := make([]treeRow, 0, len(s.Items))
		for j, it := range s.Items {
			body := itemLines(lf, th, it)
			rows = append(rows, leafRow(th, c, spad, j == len(s.Items)-1, body, it.URL))
		}
		return rows
	}
}

func leafRow(th *theme.Theme, c conn, spad string, last bool, body []string, key string) treeRow {
	iconn, ivert := c.mid, c.vert
	if last {
		iconn, ivert = c.end, c.space
	}
	lines := make([]string, len(body))
	for k, l := range body {
		if k == 0 {
			lines[k] = th.Dim.Render(spad+iconn) + l
		} else {
			lines[k] = th.Dim.Render(spad+ivert) + l
		}
	}
	return treeRow{Lines: lines, Key: key, Selectable: key != ""}
}
