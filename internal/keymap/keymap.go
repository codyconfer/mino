package keymap

import (
	"os"

	"github.com/codyconfer/viewkit/keys"

	"github.com/codyconfer/munin/internal/config"
)

const DefaultSchemeKey = "munin"

const (
	Save            keys.Action = "munin.save"
	Run             keys.Action = "munin.run"
	Delete          keys.Action = "munin.delete"
	Validate        keys.Action = "munin.validate"
	Preview         keys.Action = "munin.preview"
	Focus           keys.Action = "munin.focus"
	PluginInstall   keys.Action = "munin.plugin.install"
	PluginUninstall keys.Action = "munin.plugin.uninstall"
)

func RunBinding() keys.Binding {
	return keys.Binding{Keys: []string{"ctrl+r"}, Action: Run, Glyph: "ctrl+r", Label: "run"}
}

func DeleteBinding() keys.Binding {
	return keys.Binding{Keys: []string{"ctrl+x"}, Action: Delete, Glyph: "ctrl+x", Label: "delete"}
}

func ValidateBinding() keys.Binding {
	return keys.Binding{Keys: []string{"ctrl+t"}, Action: Validate, Glyph: "ctrl+t", Label: "validate"}
}

func PreviewBinding() keys.Binding {
	return keys.Binding{Keys: []string{"ctrl+y"}, Action: Preview, Glyph: "ctrl+y", Label: "yaml"}
}

func FocusBinding() keys.Binding {
	return keys.Binding{Keys: []string{"tab"}, Action: Focus, Glyph: "tab", Label: "focus"}
}

func BuilderBindings() []keys.Binding {
	return []keys.Binding{
		RunBinding(),
		ValidateBinding(),
		PreviewBinding(),
		DeleteBinding(),
		FocusBinding(),
	}
}

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

func Detail() *keys.Map {
	sc := keys.Cur()
	return keys.NewMap(
		sc.Binding(keys.Up),
		sc.Binding(keys.Down),
		sc.Binding(keys.PageUp),
		sc.Binding(keys.PageDown),
		sc.Binding(keys.Open),
		sc.Binding(keys.Cancel),
		sc.Binding(keys.Quit),
	)
}

func ItemList() *keys.Map {
	sc := keys.Cur()
	return keys.NewMap(
		sc.Binding(keys.Up),
		sc.Binding(keys.Down),
		sc.Binding(keys.PageUp),
		sc.Binding(keys.PageDown),
		sc.Binding(keys.Confirm),
		sc.Binding(keys.Open),
		sc.Binding(keys.Cancel),
		sc.Binding(keys.Quit),
	)
}

func ConfirmMap() *keys.Map {
	sc := keys.Cur()
	return keys.NewMap(
		sc.Binding(keys.Left),
		sc.Binding(keys.Right),
		sc.Binding(keys.Confirm),
		sc.Binding(keys.Cancel),
	)
}

func Form(extra ...keys.Binding) *keys.Map {
	sc := keys.Cur()
	bs := sc.EditorBindings(
		keys.Up, keys.Down, keys.Left, keys.Right,
		keys.Confirm, keys.Cancel, keys.Erase, keys.PageUp, keys.PageDown,
	)
	bs = append(bs, keys.Binding{Keys: []string{"ctrl+s"}, Action: Save, Glyph: "ctrl+s", Label: "save"})
	return keys.NewMap(append(bs, extra...)...)
}
