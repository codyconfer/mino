package render

import (
	"io"
	"strings"

	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/list"
	"github.com/codyconfer/viewkit/theme"
	"github.com/codyconfer/viewkit/timefmt"
	"github.com/codyconfer/viewkit/tree"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/render/glyph"
	"github.com/codyconfer/munin/internal/signals"
)

type TerminalRenderer struct{ Root string }

func (tr *TerminalRenderer) Render(w io.Writer, sections []signals.Section) error {
	if _, err := io.WriteString(w, Panels(layout.FrameFor(w), tr.Root, sections)+"\n"); err != nil {
		return errs.Wrap(errs.KindInternal, err, "write terminal output")
	}
	return nil
}

func RenderTerminalStringTitled(root string, sections []signals.Section) string {
	return treeString(FlightTree(layout.NewFrame(theme.BodyWidth), root, sections))
}

func treeString(rows []tree.Row) string {
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

const DefaultRoot = "results"

func rootLabel(root string) string {
	if strings.TrimSpace(root) == "" {
		return DefaultRoot
	}
	return root
}

func Panels(f layout.Frame, root string, sections []signals.Section) string {
	return treeString(FlightTree(f, rootLabel(root), sections))
}

func SectionItems(f layout.Frame, root string, sections []signals.Section) []list.Item {
	rows := FlightTree(f, rootLabel(root), sections)
	items := make([]list.Item, 0, len(rows))
	for _, r := range rows {
		block := strings.Join(r.Lines, "\n")
		if !r.Selectable {
			block = layout.IndentLines(block, 2)
		}
		items = append(items, list.Item{
			Block:      block,
			Key:        r.Key,
			Selectable: r.Selectable,
			GapStem:    r.GapStem,
		})
	}
	return items
}

func Success(msg string) string { return theme.Success(msg) }

func Bullet(msg string) string { return theme.Bullet(msg) }

func LoadingPanel(title, status string) string {
	return TitledBox(layout.NewFrame(theme.BodyWidth), false, title, theme.Cur().Dim.Render(status))
}

func itemLines(f layout.Frame, th *theme.Theme, it signals.Item) []string {
	icon := theme.SeverityStyle(glyph.Classify(it.Kind)).Render(glyph.Lead(glyph.ForKind(it.Kind)))
	head := icon + th.Val.Render(signals.Clean(it.Title))
	if it.Subtitle != "" {
		head += "  " + th.Dim.Render(signals.Clean(it.Subtitle))
	}
	if author := signals.Clean(it.Meta["author"]); author != "" {
		head += "  " + th.Dim.Render("@"+author)
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
