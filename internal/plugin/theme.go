package plugin

import (
	"github.com/codyconfer/viewkit/theme"

	pub "github.com/codyconfer/mino/plugin"
)

func RegisterTheme(parentID, key, displayName string, p theme.Palette) {
	pub.RegisterTheme(parentID, key, displayName, p)
}

func HasTheme(key string) bool { return pub.HasTheme(key) }
