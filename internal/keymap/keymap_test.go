package keymap

import (
	"testing"

	"github.com/codyconfer/viewkit/keys"
)

func useScheme(t *testing.T, sc keys.Scheme) {
	t.Helper()
	prev := keys.Cur()
	keys.Use(sc)
	t.Cleanup(func() { keys.Use(prev) })
}

func assertBound(t *testing.T, m *keys.Map, key string, want keys.Action) {
	t.Helper()
	got, ok := m.Action(key)
	if !ok || got != want {
		t.Errorf("Form().Action(%q) = %q,%v; want %q,true", key, got, ok, want)
	}
}

func assertUnbound(t *testing.T, m *keys.Map, key string) {
	t.Helper()
	if got, ok := m.Action(key); ok {
		t.Errorf("Form().Action(%q) = %q,true; want unbound (single-rune keys must be typeable)", key, got)
	}
}

func TestFormDefaultSchemeKeepsOnlyMultiRuneKeys(t *testing.T) {
	useScheme(t, keys.Default())
	m := Form()

	bound := map[string]keys.Action{
		"up":        keys.Up,
		"down":      keys.Down,
		"left":      keys.Left,
		"right":     keys.Right,
		"enter":     keys.Confirm,
		"spacebar":  keys.Confirm,
		"esc":       keys.Cancel,
		"backspace": keys.Erase,
		"ctrl+h":    keys.Erase,
		"pgup":      keys.PageUp,
		"pgdown":    keys.PageDown,
		"ctrl+s":    Save,
	}
	for k, want := range bound {
		assertBound(t, m, k, want)
	}

	for _, k := range []string{"k", "j", "h", "l", " ", "q", "i", "u"} {
		assertUnbound(t, m, k)
	}

	for _, a := range []keys.Action{
		keys.Up, keys.Down, keys.Left, keys.Right,
		keys.Confirm, keys.Cancel, keys.Erase, keys.PageUp, keys.PageDown, Save,
	} {
		if !m.Has(a) {
			t.Errorf("Form() missing action %q", a)
		}
	}
	if m.Has(keys.Quit) {
		t.Error("Form() must not bind Quit")
	}
}

func TestFormMinoSchemeDropsCancelAlias(t *testing.T) {
	useScheme(t, minoScheme())
	m := Form()

	assertBound(t, m, "esc", keys.Cancel)
	assertUnbound(t, m, "q")
	assertBound(t, m, "ctrl+s", Save)
}

func TestFormDropsActionsLeftWithNoKeys(t *testing.T) {
	useScheme(t, keys.Default().With(
		keys.Binding{Keys: []string{"k"}, Action: keys.Up},
		keys.Binding{Keys: []string{"tab", "x"}, Action: keys.Confirm},
		keys.Binding{Keys: []string{}, Action: keys.PageUp},
	))
	m := Form()

	if m.Has(keys.Up) {
		t.Error("Up bound to only a single-rune key must be dropped entirely")
	}
	assertUnbound(t, m, "k")
	if m.Has(keys.PageUp) {
		t.Error("PageUp with no keys must be dropped entirely")
	}
	assertBound(t, m, "tab", keys.Confirm)
	assertUnbound(t, m, "x")
	assertBound(t, m, "down", keys.Down)
}

func TestFormAppendsExtraBindingsVerbatim(t *testing.T) {
	useScheme(t, minoScheme())
	m := Form(BuilderBindings()...)

	extras := map[string]keys.Action{
		"ctrl+r": Run,
		"ctrl+t": Validate,
		"ctrl+y": Preview,
		"ctrl+x": Delete,
		"tab":    Focus,
	}
	for k, want := range extras {
		assertBound(t, m, k, want)
	}
	assertBound(t, m, "ctrl+s", Save)
}

func TestFormExtraSingleRuneKeysAreNotStripped(t *testing.T) {
	useScheme(t, minoScheme())
	m := Form(keys.Binding{Keys: []string{"d"}, Action: Delete})
	assertBound(t, m, "d", Delete)
}

func TestBuilderBindingsFollowTheActiveScheme(t *testing.T) {
	useScheme(t, minoScheme().With(
		keys.Binding{Keys: []string{"ctrl+e"}, Action: Run, Glyph: "ctrl+e", Label: "run"},
	))

	if got := RunBinding().Keys; len(got) != 1 || got[0] != "ctrl+e" {
		t.Fatalf("RunBinding().Keys = %v, want the scheme's ctrl+e", got)
	}
	m := Form(BuilderBindings()...)
	assertBound(t, m, "ctrl+e", Run)
	assertUnbound(t, m, "ctrl+r")
	assertBound(t, m, "ctrl+t", Validate)
}

func TestBuilderBindingsFallBackWhenASchemeOmitsThem(t *testing.T) {
	useScheme(t, keys.Default())

	for _, tc := range []struct {
		binding keys.Binding
		key     string
	}{
		{RunBinding(), "ctrl+r"},
		{SaveBinding(), "ctrl+s"},
		{ValidateBinding(), "ctrl+t"},
		{DeleteBinding(), "ctrl+x"},
		{CopyBinding(), "ctrl+g"},
		{WriteBinding(), "ctrl+w"},
	} {
		if len(tc.binding.Keys) == 0 || tc.binding.Keys[0] != tc.key {
			t.Errorf("%q binding = %v, want the mino default %q so the editor stays usable",
				tc.binding.Action, tc.binding.Keys, tc.key)
		}
	}
}
