package plugin

import (
	"github.com/codyconfer/viewkit/theme"

	pub "github.com/codyconfer/munin/plugin"
)

// RegisterTheme registers a theme palette and KindTheme descriptor.
func RegisterTheme(parentID, key, displayName string, p theme.Palette) {
	pub.RegisterTheme(parentID, key, displayName, p)
}

// HasTheme reports whether key is registered in the theme registry.
func HasTheme(key string) bool { return pub.HasTheme(key) }
