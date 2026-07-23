package keymap

import (
	"os"
	"slices"

	"github.com/codyconfer/viewkit/keys"

	"github.com/codyconfer/munin/internal/config"
)

const DefaultSchemeKey = "munin"

const (
	Save keys.Action = "munin.save"
	Yes  keys.Action = "munin.yes"
	No   keys.Action = "munin.no"
)

func muninScheme() keys.Scheme {
	return keys.Default().With(
		keys.Binding{Keys: []string{"ctrl+s"}, Action: Save, Glyph: "ctrl+s", Label: "save"},
		keys.Binding{Keys: []string{"esc", "q"}, Action: keys.Cancel, Glyph: "esc", Label: "back"},
		keys.Binding{Keys: []string{"ctrl+c"}, Action: keys.Quit, Glyph: "ctrl+c", Label: "quit"},
		keys.Binding{Keys: []string{"y", "Y"}, Action: Yes, Glyph: "y", Label: "yes"},
		keys.Binding{Keys: []string{"n", "N"}, Action: No, Glyph: "n", Label: "no"},
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

func Form(extra ...keys.Binding) *keys.Map {
	sc := keys.Cur()
	bs := editorBindings(sc,
		keys.Up, keys.Down, keys.Left, keys.Right,
		keys.Confirm, keys.Cancel, keys.Erase, keys.PageUp, keys.PageDown,
	)
	bs = append(bs, keys.Binding{Keys: []string{"ctrl+s"}, Action: Save, Glyph: "ctrl+s", Label: "save"})
	return keys.NewMap(append(bs, extra...)...)
}

func Confirm() *keys.Map {
	sc := keys.Cur()
	return keys.NewMap(
		sc.Binding(keys.Left),
		withKeys(sc.Binding(keys.Right), "tab"),
		sc.Binding(keys.Confirm),
		sc.Binding(keys.Cancel),
		sc.Binding(keys.Quit),
		keys.Binding{Keys: []string{"y", "Y"}, Action: Yes, Glyph: "y", Label: "yes"},
		keys.Binding{Keys: []string{"n", "N"}, Action: No, Glyph: "n", Label: "no"},
	)
}

func IsQuit(input string) bool {
	return slices.Contains(keys.Cur().Binding(keys.Quit).Keys, input)
}

func Hint(a keys.Action) [2]string {
	b := keys.Cur().Binding(a)
	return [2]string{b.DisplayGlyph(), b.Label}
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

func withKeys(b keys.Binding, extra ...string) keys.Binding {
	b.Keys = append(append([]string{}, b.Keys...), extra...)
	return b
}
