package render

import (
	"io"

	"github.com/charmbracelet/lipgloss"

	"github.com/codyconfer/viewkit/glyph"
	"github.com/codyconfer/viewkit/ui"
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
	Glyphs  glyph.Set
}

// NewReportStyles builds report styles for w from scope (nil falls back to
// the process defaults).
func NewReportStyles(w io.Writer, scope *ui.Scope) ReportStyles {
	if scope == nil {
		scope = ui.Default()
	}
	r := lipgloss.NewRenderer(w)
	th := scope.Theme
	return ReportStyles{
		Title:   r.NewStyle().Bold(true).Underline(true),
		OK:      r.NewStyle().Foreground(th.SeverityColor(glyph.SeverityPositive)),
		Err:     r.NewStyle().Foreground(th.SeverityColor(glyph.SeverityNegative)).Bold(true),
		Warn:    r.NewStyle().Foreground(th.SeverityColor(glyph.SeverityWarning)),
		Name:    r.NewStyle().Bold(true),
		Dim:     r.NewStyle().Faint(true),
		Snippet: r.NewStyle().Faint(true),
		Fix:     r.NewStyle().Foreground(th.Accent.GetForeground()),
		Glyphs:  scope.Glyphs,
	}
}
