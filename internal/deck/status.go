package deck

import (
	"context"

	"github.com/charmbracelet/lipgloss"

	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"
)

type StatusLevel = theme.Severity

const (
	StatusMuted = theme.SevMuted
	StatusOK    = theme.SevOK
	StatusWarn  = theme.SevWarn
	StatusBad   = theme.SevBad
)

type ServiceStatus struct {
	Name   string
	Detail string
	Level  StatusLevel
}

type StatusInfo struct {
	GitHubUser      string
	SigningVerified bool
	Services        []ServiceStatus
}

type StatusFunc func(context.Context) StatusInfo

func glyphForLevel(level StatusLevel) string { return theme.SeverityGlyph(level) }

func statusColor(level StatusLevel) lipgloss.TerminalColor { return theme.SeverityColor(level) }

func stripText(fg lipgloss.TerminalColor, s string) string { return theme.StripText(fg, s) }

func stripBold(fg lipgloss.TerminalColor, s string) string { return theme.StripBold(fg, s) }

func stripBlock(width int, lines ...string) string { return theme.StripBlock(width, lines...) }

func stripLine(width int, left, right string) string {
	return layout.SpreadBG(theme.StripBg(), left, right, width)
}
