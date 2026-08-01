package keymap

import (
	"os"

	"github.com/codyconfer/viewkit/keys"

	"github.com/codyconfer/mino/internal/config"
)

const DefaultSchemeKey = "mino"

const (
	Save            keys.Action = "mino.save"
	Run             keys.Action = "mino.run"
	Delete          keys.Action = "mino.delete"
	Validate        keys.Action = "mino.validate"
	Preview         keys.Action = "mino.preview"
	Focus           keys.Action = "mino.focus"
	Copy            keys.Action = "mino.copy"
	Write           keys.Action = "mino.write"
	PluginInstall   keys.Action = "mino.plugin.install"
	PluginUninstall keys.Action = "mino.plugin.uninstall"
	Toggle          keys.Action = "mino.toggle"
)

func minoBindings() []keys.Binding {
	return []keys.Binding{
		{Keys: []string{"ctrl+r"}, Action: Run, Glyph: "ctrl+r", Label: "run"},
		{Keys: []string{"ctrl+x"}, Action: Delete, Glyph: "ctrl+x", Label: "delete"},
		{Keys: []string{"ctrl+t"}, Action: Validate, Glyph: "ctrl+t", Label: "validate"},
		{Keys: []string{"ctrl+y"}, Action: Preview, Glyph: "ctrl+y", Label: "yaml"},
		{Keys: []string{"tab"}, Action: Focus, Glyph: "tab", Label: "focus"},
		{Keys: []string{"ctrl+g"}, Action: Copy, Glyph: "ctrl+g", Label: "copy"},
		{Keys: []string{"ctrl+w"}, Action: Write, Glyph: "ctrl+w", Label: "write"},
		{Keys: []string{"ctrl+s"}, Action: Save, Glyph: "ctrl+s", Label: "save"},
		{Keys: []string{"i"}, Action: PluginInstall, Glyph: "i", Label: "install"},
		{Keys: []string{"u"}, Action: PluginUninstall, Glyph: "u", Label: "uninstall"},
		{Keys: []string{"x"}, Action: Toggle, Glyph: "x", Label: "done"},
	}
}

func BuilderBindings() []keys.Binding {
	sc := keys.Cur()
	actions := []keys.Action{Run, Validate, Preview, Delete, Focus, Copy, Write}
	out := make([]keys.Binding, 0, len(actions))
	for _, a := range actions {
		out = append(out, sc.Binding(a))
	}
	return out
}

func minoScheme() keys.Scheme {
	return keys.Default().With(append(minoBindings(),
		keys.Binding{Keys: []string{"esc", "q"}, Action: keys.Cancel, Glyph: "esc", Label: "back"},
		keys.Binding{Keys: []string{"ctrl+c"}, Action: keys.Quit, Glyph: "ctrl+c", Label: "quit"},
	)...)
}

func Register() {
	keys.Register(DefaultSchemeKey, "Mino", minoScheme())
}

// UseNamed activates the registered scheme for key, grafting mino's app
// bindings onto it so every action keymap defines stays reachable.
func UseNamed(key string) bool {
	sc, ok := keys.Named(key)
	if !ok {
		return false
	}
	keys.Use(sc.WithDefaults(minoBindings()...))
	return true
}

func Install() {
	Register()
	keys.Use(minoScheme())
	key := os.Getenv("MINO_KEYS")
	if key == "" {
		key = config.LoadGlobalSettings().Keys
	}
	if key == "" || key == DefaultSchemeKey {
		return
	}
	UseNamed(key)
}

func Menu() *keys.Map {
	return keys.MapFor(keys.Up, keys.Down, keys.Confirm, keys.Cancel, keys.Quit)
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
		sc.Binding(PluginInstall),
		sc.Binding(PluginUninstall),
	)
}

func Detail() *keys.Map {
	return keys.MapFor(keys.Up, keys.Down, keys.PageUp, keys.PageDown, keys.Open, keys.Cancel, keys.Quit)
}

func ItemList() *keys.Map {
	return keys.MapFor(keys.Up, keys.Down, keys.PageUp, keys.PageDown, keys.Confirm, keys.Open, keys.Cancel, keys.Quit)
}

func ConfirmMap() *keys.Map {
	return keys.MapFor(keys.Left, keys.Right, keys.Confirm, keys.Cancel)
}

func Form(extra ...keys.Binding) *keys.Map {
	sc := keys.Cur()
	bs := sc.EditorBindings(
		keys.Complete, keys.CompleteNext, keys.CompletePrev,
		keys.Up, keys.Down, keys.Left, keys.Right,
		keys.Confirm, keys.Cancel, keys.Erase, keys.PageUp, keys.PageDown,
	)
	if b := sc.Binding(Save); len(b.Keys) > 0 {
		bs = append(bs, b)
	}
	return keys.NewMap(append(bs, extra...)...)
}
