package render

import (
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/list"
	"github.com/codyconfer/viewkit/theme"
	"github.com/codyconfer/viewkit/timefmt"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/render/glyph"
	"github.com/codyconfer/munin/internal/signals"
)

type TerminalRenderer struct{}

func (tr *TerminalRenderer) Render(w io.Writer, sections []signals.Section) error {
	if _, err := io.WriteString(w, Panels(layout.FrameFor(w), sections)+"\n"); err != nil {
		return errs.Wrap(errs.KindInternal, err, "write terminal output")
	}
	return nil
}

func RenderTerminalStringTitled(root string, sections []signals.Section) string {
	return treeString(FlightTree(layout.NewFrame(theme.BodyWidth), root, sections))
}

func treeString(rows []treeRow) string {
	lines := make([]string, 0, len(rows))
	for _, r := range rows {
		lines = append(lines, r.Lines...)
	}
	return strings.Join(lines, "\n")
}

func sectionGlyph(s signals.Section) string {
	n := strings.ToLower(s.Signal + " " + s.Title)
	switch {
	case strings.Contains(n, "github"):
		return glyph.GitHub()
	case strings.Contains(n, "slack"):
		return glyph.Slack()
	case strings.Contains(n, "cal"), strings.Contains(n, "gmail"), strings.Contains(n, "mail"),
		strings.Contains(n, "doc"), strings.Contains(n, "drive"), strings.Contains(n, "task"),
		strings.Contains(n, "google"):
		return glyph.Google()
	}
	return glyph.Bullet()
}

func Panels(f layout.Frame, sections []signals.Section) string {
	return treeString(FlightTree(f, "flight", sections))
}

func SectionItems(f layout.Frame, sections []signals.Section) []list.Item {
	rows := FlightTree(f, "flight", sections)
	items := make([]list.Item, 0, len(rows))
	for _, r := range rows {
		block := strings.Join(r.Lines, "\n")
		if !r.Selectable {
			block = indentLines(block, "  ")
		}
		items = append(items, list.Item{Block: block, Key: r.Key, Selectable: r.Selectable})
	}
	return items
}

func indentLines(block, pad string) string {
	lines := strings.Split(block, "\n")
	for i, l := range lines {
		lines[i] = pad + l
	}
	return strings.Join(lines, "\n")
}

func Success(msg string) string { return theme.Success(msg) }

func Bullet(msg string) string { return theme.Bullet(msg) }

func LoadingPanel(title, status string) string {
	return TitledBox(layout.NewFrame(theme.BodyWidth), false, title, theme.Cur().Dim.Render(status))
}

func kindStyle(th *theme.Theme, kind string) lipgloss.Style {
	switch glyph.Classify(kind) {
	case glyph.KindPositive:
		return th.Can
	case glyph.KindWarning:
		if len(th.Series) > 2 {
			return th.Series[2]
		}
		return th.Cant
	case glyph.KindNegative:
		return th.Cant
	default:
		return th.Dim
	}
}

func itemLines(f layout.Frame, th *theme.Theme, it signals.Item) []string {
	icon := kindStyle(th, it.Kind).Render(glyph.Lead(glyph.ForKind(it.Kind)))
	head := icon + th.Val.Render(signals.Clean(it.Title))
	if it.Subtitle != "" {
		head += "  " + th.Dim.Render(signals.Clean(it.Subtitle))
	}
	var lines []string
	if !it.Timestamp.IsZero() {
		lines = append(lines, f.Spread(head, th.Dim.Render(timefmt.Rel(it.Timestamp))))
	} else {
		lines = append(lines, head)
	}
	if it.URL != "" {
		lines = append(lines, th.Dim.Render(signals.Clean(it.URL)))
	}
	return lines
}
