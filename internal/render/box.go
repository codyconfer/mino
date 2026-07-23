package render

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"
)

func TitledBox(f layout.Frame, focused bool, title string, lines ...string) string {
	th := theme.Cur()
	inner := f.BodyWidth()
	span := inner + 2

	border := th.Dim
	if focused {
		border = th.Accent
	}

	out := make([]string, 0, len(lines)+2)
	out = append(out, topBorder(border, th.PanelTitle, title, span))

	edge := border.Render("│")
	for _, ln := range lines {
		for sub := range strings.SplitSeq(ansi.Hardwrap(ln, inner, false), "\n") {
			pad := max(inner-ansi.StringWidth(sub), 0)
			out = append(out, edge+" "+sub+strings.Repeat(" ", pad)+" "+edge)
		}
	}

	out = append(out, border.Render("╰"+strings.Repeat("─", span)+"╯"))
	return strings.Join(out, "\n")
}

func topBorder(border, titleSty lipgloss.Style, title string, span int) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return border.Render("╭" + strings.Repeat("─", span) + "╮")
	}
	seg := " " + ansi.Truncate(title, span-2, "…") + " "
	fill := max(span-ansi.StringWidth(seg), 0)
	return border.Render("╭") + titleSty.Render(seg) + border.Render(strings.Repeat("─", fill)+"╮")
}
