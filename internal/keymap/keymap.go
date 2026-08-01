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

func binding(a keys.Action) keys.Binding {
	if b := keys.Cur().Binding(a); len(b.Keys) > 0 {
		return b
	}
	for _, b := range minoBindings() {
		if b.Action == a {
			return b
		}
	}
	return keys.Binding{Action: a}
}

func RunBinding() keys.Binding      { return binding(Run) }
func DeleteBinding() keys.Binding   { return binding(Delete) }
func ValidateBinding() keys.Binding { return binding(Validate) }
func PreviewBinding() keys.Binding  { return binding(Preview) }
func FocusBinding() keys.Binding    { return binding(Focus) }
func CopyBinding() keys.Binding     { return binding(Copy) }
func WriteBinding() keys.Binding    { return binding(Write) }
func SaveBinding() keys.Binding     { return binding(Save) }
func ToggleBinding() keys.Binding   { return binding(Toggle) }

func BuilderBindings() []keys.Binding {
	return []keys.Binding{
		RunBinding(),
		ValidateBinding(),
		PreviewBinding(),
		DeleteBinding(),
		FocusBinding(),
		CopyBinding(),
		WriteBinding(),
	}
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
		binding(PluginInstall),
		binding(PluginUninstall),
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
		keys.Complete, keys.CompleteNext, keys.CompletePrev,
		keys.Up, keys.Down, keys.Left, keys.Right,
		keys.Confirm, keys.Cancel, keys.Erase, keys.PageUp, keys.PageDown,
	)
	bs = append(bs, SaveBinding())
	return keys.NewMap(append(bs, extra...)...)
}
