package keymap

import (
	"os"

	"github.com/codyconfer/viewkit/keys"

	"github.com/codyconfer/munin/internal/config"
)

const DefaultSchemeKey = "munin"

const (
	Save            keys.Action = "munin.save"
	PluginInstall   keys.Action = "munin.plugin.install"
	PluginUninstall keys.Action = "munin.plugin.uninstall"
)

func muninScheme() keys.Scheme {
	return keys.Default().With(
		keys.Binding{Keys: []string{"ctrl+s"}, Action: Save, Glyph: "ctrl+s", Label: "save"},
		keys.Binding{Keys: []string{"i"}, Action: PluginInstall, Glyph: "i", Label: "install"},
		keys.Binding{Keys: []string{"u"}, Action: PluginUninstall, Glyph: "u", Label: "uninstall"},
		keys.Binding{Keys: []string{"esc", "q"}, Action: keys.Cancel, Glyph: "esc", Label: "back"},
		keys.Binding{Keys: []string{"ctrl+c"}, Action: keys.Quit, Glyph: "ctrl+c", Label: "quit"},
	)
}

func Register() {
	keys.Register(DefaultSchemeKey, "Munin", muninScheme())
}

func Install() {
	Register()
	keys.Use(muninScheme())
	key := os.Getenv("MUNIN_KEYS")
	if key == "" {
		key = config.LoadGlobalSettings().Keys
	}
	if key == "" || key == DefaultSchemeKey {
		return
	}
	if sc, ok := keys.Named(key); ok {
		keys.Use(sc)
	}
}

// Menu returns nav bindings from the active scheme (viewkit/keys).
func Menu() *keys.Map {
	sc := keys.Cur()
	return keys.NewMap(
		sc.Binding(keys.Up),
		sc.Binding(keys.Down),
		sc.Binding(keys.Confirm),
		sc.Binding(keys.Cancel),
		sc.Binding(keys.Quit),
	)
}

// Plugins returns Menu bindings plus enable/disable, install, and uninstall.
// Enter/d toggles enablement (plugin stays listed); u uninstalls (removes from list).
func Plugins() *keys.Map {
	sc := keys.Cur()
	confirm := sc.Binding(keys.Confirm)
	confirm.Keys = append(append([]string{}, confirm.Keys...), "d")
	confirm.Glyph = "enter/d"
	confirm.Label = "enable/disable"
	return keys.NewMap(
		sc.Binding(keys.Up),
		sc.Binding(keys.Down),
		confirm,
		sc.Binding(keys.Cancel),
		sc.Binding(keys.Quit),
		keys.Binding{Keys: []string{"i"}, Action: PluginInstall, Glyph: "i", Label: "install"},
		keys.Binding{Keys: []string{"u"}, Action: PluginUninstall, Glyph: "u", Label: "uninstall"},
	)
}

// Form returns editor-safe bindings plus munin Save.
func Form(extra ...keys.Binding) *keys.Map {
	sc := keys.Cur()
	bs := editorBindings(sc,
		keys.Up, keys.Down, keys.Left, keys.Right,
		keys.Confirm, keys.Cancel, keys.Erase, keys.PageUp, keys.PageDown,
	)
	bs = append(bs, keys.Binding{Keys: []string{"ctrl+s"}, Action: Save, Glyph: "ctrl+s", Label: "save"})
	return keys.NewMap(append(bs, extra...)...)
}

func editorBindings(sc keys.Scheme, actions ...keys.Action) []keys.Binding {
	out := make([]keys.Binding, 0, len(actions))
	for _, a := range actions {
		b := sc.Binding(a)
		b.Keys = controlKeys(b.Keys)
		if len(b.Keys) > 0 {
			out = append(out, b)
		}
	}
	return out
}

func controlKeys(in []string) []string {
	out := make([]string, 0, len(in))
	for _, k := range in {
		if len([]rune(k)) > 1 {
			out = append(out, k)
		}
	}
	return out
}
