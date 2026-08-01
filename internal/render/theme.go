package render

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/mino/internal/plugin"
)

const DefaultThemeKey = "mino"

var minoPalette = theme.Palette{
	Accent:   lipgloss.Color("#ff8c42"),
	Border:   lipgloss.Color("#33405a"),
	Muted:    lipgloss.Color("#7d8aa3"),
	Text:     lipgloss.Color("#e2eaf7"),
	Selected: lipgloss.Color("#ffb066"),
	Success:  lipgloss.Color("#3ddc84"),
	Warning:  lipgloss.Color("#ffbe4d"),
	Failure:  lipgloss.Color("#ff5c57"),
	Info:     lipgloss.Color("#5aa9ff"),
	Series2:  lipgloss.Color("#c58aff"),
	Series3:  lipgloss.Color("#33d0c4"),
	Bg:       lipgloss.Color("#0b0f16"),
}

func init() {
	plugin.RegisterTheme("", DefaultThemeKey, "Mino", minoPalette)
}

func InstallDefaultTheme() {
	if t, ok := theme.Named(DefaultThemeKey); ok {
		theme.Use(t)
	}
}
