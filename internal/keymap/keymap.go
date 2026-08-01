package keymap

import (
	"os"

	"github.com/codyconfer/viewkit/keys"

	"github.com/codyconfer/mino/internal/config"
)

// Register adds mino's scheme to the keys registry.
func Register() {
	keys.Register(DefaultSchemeKey, "Mino", minoScheme())
}

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

// BuilderBindings returns sc's bindings for the query-builder actions.
func BuilderBindings(sc keys.Scheme) []keys.Binding {
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

// SchemeFor returns the registered scheme for key grafted with mino's app
// bindings, falling back to mino's own scheme for unknown keys.
func SchemeFor(key string) keys.Scheme {
	if sc, ok := keys.Named(key); ok {
		return sc.WithDefaults(minoBindings()...)
	}
	return minoScheme()
}

// SchemeKey returns the effective key-scheme key: MINO_KEYS over settings.
func SchemeKey() string {
	if key := os.Getenv("MINO_KEYS"); key != "" {
		return key
	}
	return config.LoadGlobalSettings().Keys
}

func Menu(sc keys.Scheme) *keys.Map {
	return sc.MapFor(keys.Up, keys.Down, keys.Confirm, keys.Cancel, keys.Quit)
}

func Plugins(sc keys.Scheme) *keys.Map {
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

func Detail(sc keys.Scheme) *keys.Map {
	return sc.MapFor(keys.Up, keys.Down, keys.PageUp, keys.PageDown, keys.Open, keys.Cancel, keys.Quit)
}

func ItemList(sc keys.Scheme) *keys.Map {
	return sc.MapFor(keys.Up, keys.Down, keys.PageUp, keys.PageDown, keys.Confirm, keys.Open, keys.Cancel, keys.Quit)
}

func ConfirmMap(sc keys.Scheme) *keys.Map {
	return sc.MapFor(keys.Left, keys.Right, keys.Confirm, keys.Cancel)
}

func Form(sc keys.Scheme, extra ...keys.Binding) *keys.Map {
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
