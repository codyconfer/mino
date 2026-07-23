package render

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/codyconfer/viewkit/theme"
)

const DefaultThemeKey = "munin"

var muninPalette = theme.Palette{
	Accent:   lipgloss.Color("#ff8c42"),
	Border:   lipgloss.Color("#2b3440"),
	Muted:    lipgloss.Color("#6e7c91"),
	Text:     lipgloss.Color("#c9d4e3"),
	Selected: lipgloss.Color("#ffb066"),
	Success:  lipgloss.Color("#4a9edb"),
	Warning:  lipgloss.Color("#ffb454"),
	Failure:  lipgloss.Color("#ff6b5e"),
	Info:     lipgloss.Color("#5aa9ff"),
	Series2:  lipgloss.Color("#8fb8ff"),
	Bg:       lipgloss.Color("#0b0f16"),
}

func RegisterThemes() {
	theme.Register(DefaultThemeKey, "Munin", muninPalette)
}

func InstallDefaultTheme() {
	RegisterThemes()
	if t, ok := theme.Named(DefaultThemeKey); ok {
		theme.Use(t)
	}
}
