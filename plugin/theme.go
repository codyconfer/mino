package plugin

import (
	"github.com/codyconfer/viewkit/theme"
)

// RegisterTheme registers a viewkit theme palette and a KindTheme descriptor
// (Ref = key). Empty parentID makes the theme a primary enableable plugin.
func RegisterTheme(parentID, key, displayName string, p theme.Palette) {
	if key == "" {
		panic("plugin: RegisterTheme requires key")
	}
	if _, ok := ByKind(KindTheme, key); ok {
		return
	}
	theme.Register(key, displayName, p)
	id := "theme." + key
	if parentID != "" {
		id = parentID + "/theme/" + key
	}
	Register(Descriptor{
		ID:     id,
		Kind:   KindTheme,
		Ref:    key,
		Parent: parentID,
	})
}

// HasTheme reports whether key is registered in the theme registry.
func HasTheme(key string) bool {
	_, ok := theme.Named(key)
	return ok
}
