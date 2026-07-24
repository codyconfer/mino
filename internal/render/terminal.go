package render

import (
	"fmt"
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

func RenderTerminalString(sections []signals.Section) string {
	return Panels(layout.NewFrame(theme.BodyWidth), sections)
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
	th := theme.Cur()
	blocks := make([]string, 0, len(sections))
	for i, s := range sections {
		title := s.Title
		if title == "" {
			title = s.Signal
		}
		title = fmt.Sprintf("%s  (%d)", signals.Clean(title), len(s.Items))
		icon := th.Series[i%len(th.Series)].Render(glyph.Lead(sectionGlyph(s)))

		var lines []string
		switch {
		case s.Err != nil:
			errLines := strings.Split(signals.Clean(s.Err.Error()), "\n")
			lines = append(lines, th.Cant.Render(glyph.Warn()+" "+errLines[0]))
			for _, l := range errLines[1:] {
				lines = append(lines, th.Dim.Render(l))
			}
		case len(s.Items) == 0:
			lines = append(lines, th.Dim.Render("nothing to show"))
		default:
			for _, it := range s.Items {
				lines = append(lines, itemLines(f, th, it)...)
			}
		}
		blocks = append(blocks, TitledBoxIcon(f, false, icon, title, lines...))
	}
	return layout.StackTight(blocks...)
}

func SectionItems(f layout.Frame, sections []signals.Section) []list.Item {
	th := theme.Cur()
	var items []list.Item
	for i, s := range sections {
		title := s.Title
		if title == "" {
			title = s.Signal
		}
		title = fmt.Sprintf("%s  (%d)", signals.Clean(title), len(s.Items))
		icon := th.Series[i%len(th.Series)].Render(glyph.Lead(sectionGlyph(s)))
		items = append(items, list.Item{Block: icon + th.Title.Render(title)})

		switch {
		case s.Err != nil:
			errLines := strings.Split(signals.Clean(s.Err.Error()), "\n")
			block := []string{th.Cant.Render(glyph.Warn() + " " + errLines[0])}
			for _, l := range errLines[1:] {
				block = append(block, th.Dim.Render(l))
			}
			items = append(items, list.Item{Block: strings.Join(block, "\n")})
		case len(s.Items) == 0:
			items = append(items, list.Item{Block: th.Dim.Render("nothing to show")})
		default:
			for _, it := range s.Items {
				items = append(items, list.Item{
					Block:      strings.Join(itemLines(f, th, it), "\n"),
					Key:        it.URL,
					Selectable: it.URL != "",
				})
			}
		}
	}
	return items
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
