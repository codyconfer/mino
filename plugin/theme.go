package plugin

import (
	"github.com/codyconfer/viewkit/theme"
)

func RegisterTheme(parentID, key, displayName string, p theme.Palette) {
	if key == "" {
		noteDiagnostic(Diagnostic{
			PluginID: parentID,
			Kind:     KindTheme,
			Message:  "RegisterTheme requires a non-empty key; theme skipped",
		})
		return
	}
	if prev, ok := ByKind(KindTheme, key); ok {
		if prev.Parent != parentID && prev.ID != parentID {
			noteDiagnosticf(parentID, KindTheme, key,
				"theme %q is already owned by %q; later theme skipped", key, prev.ID)
		}
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
