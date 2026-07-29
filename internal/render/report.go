package render

import (
	"io"

	"github.com/charmbracelet/lipgloss"

	"github.com/codyconfer/viewkit/glyph"
	"github.com/codyconfer/viewkit/theme"
)

type ReportStyles struct {
	Title   lipgloss.Style
	OK      lipgloss.Style
	Err     lipgloss.Style
	Warn    lipgloss.Style
	Name    lipgloss.Style
	Dim     lipgloss.Style
	Snippet lipgloss.Style
	Fix     lipgloss.Style
}

func NewReportStyles(w io.Writer) ReportStyles {
	r := lipgloss.NewRenderer(w)
	th := theme.Cur()
	return ReportStyles{
		Title:   r.NewStyle().Bold(true).Underline(true),
		OK:      r.NewStyle().Foreground(theme.SeverityColor(glyph.SeverityPositive)),
		Err:     r.NewStyle().Foreground(theme.SeverityColor(glyph.SeverityNegative)).Bold(true),
		Warn:    r.NewStyle().Foreground(theme.SeverityColor(glyph.SeverityWarning)),
		Name:    r.NewStyle().Bold(true),
		Dim:     r.NewStyle().Faint(true),
		Snippet: r.NewStyle().Faint(true),
		Fix:     r.NewStyle().Foreground(th.Accent.GetForeground()),
	}
}
