package app

import (
	"os"

	"github.com/codyconfer/viewkit/theme"
	"github.com/codyconfer/viewkit/ui"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/keymap"
	"github.com/codyconfer/mino/internal/render"
)

// BuildScope assembles the scoped rendering context for themeKey/keysKey,
// falling back to mino's own theme and scheme for empty or unknown keys.
func BuildScope(themeKey, keysKey string) *ui.Scope {
	s := ui.Default()
	if t, ok := theme.Named(themeKey); ok {
		s.Theme = t
	} else {
		s.Theme = render.DefaultTheme()
	}
	s.Keys = keymap.SchemeFor(keysKey)
	return s
}

// ThemeKey returns the effective theme key: MINO_THEME over settings.
func ThemeKey() string {
	if key := os.Getenv("MINO_THEME"); key != "" {
		return key
	}
	return config.LoadGlobalSettings().Theme
}

// KeysKey returns the effective key-scheme key: MINO_KEYS over settings.
func KeysKey() string { return keymap.SchemeKey() }
