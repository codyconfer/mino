package render

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"
	"github.com/codyconfer/viewkit/timefmt"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/signals"
)

type TerminalRenderer struct{}

func (tr *TerminalRenderer) Render(w io.Writer, sections []signals.Section) error {
	if _, err := io.WriteString(w, Panels(frameFor(w), sections)+"\n"); err != nil {
		return errs.Wrap(errs.KindInternal, err, "write terminal output")
	}
	return nil
}

func RenderTerminalString(sections []signals.Section) string {
	return Panels(layout.NewFrame(theme.BodyWidth), sections)
}

func Panels(f layout.Frame, sections []signals.Section) string {
	th := theme.Cur()
	blocks := make([]string, 0, len(sections))
	for _, s := range sections {
		title := s.Title
		if title == "" {
			title = s.Signal
		}
		title = fmt.Sprintf("%s  (%d)", title, len(s.Items))

		var lines []string
		switch {
		case s.Err != nil:
			errLines := strings.Split(s.Err.Error(), "\n")
			lines = append(lines, th.Cant.Render("⚠ "+errLines[0]))
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
		blocks = append(blocks, f.Panel(title, lines...))
	}
	return layout.Stack(blocks...)
}

func LoadingPanel(title, status string) string {
	return layout.NewFrame(theme.BodyWidth).Panel(title, theme.Cur().Dim.Render(status))
}

func itemLines(f layout.Frame, th *theme.Theme, it signals.Item) []string {
	head := th.Val.Render(it.Title)
	if it.Subtitle != "" {
		head += "  " + th.Dim.Render(it.Subtitle)
	}
	var lines []string
	if !it.Timestamp.IsZero() {
		lines = append(lines, f.Spread(head, th.Dim.Render(timefmt.Rel(it.Timestamp))))
	} else {
		lines = append(lines, head)
	}
	if it.URL != "" {
		lines = append(lines, th.Dim.Render(it.URL))
	}
	return lines
}

func frameFor(w io.Writer) layout.Frame {
	if f, ok := w.(*os.File); ok {
		if width, _, err := term.GetSize(int(f.Fd())); err == nil && width > 0 {
			return layout.ScreenFrame(width)
		}
	}
	return layout.NewFrame(theme.BodyWidth)
}
