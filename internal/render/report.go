package render

import (
	"io"

	"github.com/charmbracelet/lipgloss"
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
	return ReportStyles{
		Title:   r.NewStyle().Bold(true).Underline(true),
		OK:      r.NewStyle().Foreground(lipgloss.Color("10")),
		Err:     r.NewStyle().Foreground(lipgloss.Color("9")).Bold(true),
		Warn:    r.NewStyle().Foreground(lipgloss.Color("11")),
		Name:    r.NewStyle().Bold(true),
		Dim:     r.NewStyle().Faint(true),
		Snippet: r.NewStyle().Faint(true),
		Fix:     r.NewStyle().Foreground(lipgloss.Color("12")),
	}
}
