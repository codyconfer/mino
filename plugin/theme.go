package plugin

import (
	"github.com/codyconfer/viewkit/theme"
)

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

func HasTheme(key string) bool {
	_, ok := theme.Named(key)
	return ok
}
