package keymap

import (
	"testing"

	"github.com/codyconfer/viewkit/keys"
)

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
	m := Form(keys.Default().WithDefaults(minoBindings()...))

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

	for _, k := range []string{"k", "j", "h", "l", " ", "q", "i", "u", "x", "e", "f"} {
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
	m := Form(minoScheme())

	assertBound(t, m, "esc", keys.Cancel)
	assertUnbound(t, m, "q")
	assertBound(t, m, "ctrl+s", Save)
}

func TestFormDropsActionsLeftWithNoKeys(t *testing.T) {
	m := Form(keys.Default().With(
		keys.Binding{Keys: []string{"k"}, Action: keys.Up},
		keys.Binding{Keys: []string{"tab", "x"}, Action: keys.Confirm},
		keys.Binding{Keys: []string{}, Action: keys.PageUp},
	))

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
	sc := minoScheme()
	m := Form(sc, BuilderBindings(sc)...)

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
	m := Form(minoScheme(), keys.Binding{Keys: []string{"d"}, Action: Delete})
	assertBound(t, m, "d", Delete)
}

func TestBuilderBindingsFollowTheGivenScheme(t *testing.T) {
	sc := minoScheme().With(
		keys.Binding{Keys: []string{"ctrl+e"}, Action: Run, Glyph: "ctrl+e", Label: "run"},
	)

	if got := sc.Binding(Run).Keys; len(got) != 1 || got[0] != "ctrl+e" {
		t.Fatalf("Binding(Run).Keys = %v, want the scheme's ctrl+e", got)
	}
	m := Form(sc, BuilderBindings(sc)...)
	assertBound(t, m, "ctrl+e", Run)
	assertUnbound(t, m, "ctrl+r")
	assertBound(t, m, "ctrl+t", Validate)
}

func TestSchemeForFillsBindingsASchemeOmits(t *testing.T) {
	sc := keys.Default().WithDefaults(minoBindings()...)

	for _, tc := range []struct {
		action keys.Action
		key    string
	}{
		{Run, "ctrl+r"},
		{Save, "ctrl+s"},
		{Validate, "ctrl+t"},
		{Delete, "ctrl+x"},
		{Copy, "ctrl+g"},
		{Write, "ctrl+w"},
	} {
		b := sc.Binding(tc.action)
		if len(b.Keys) == 0 || b.Keys[0] != tc.key {
			t.Errorf("%q binding = %v, want the mino default %q so the editor stays usable",
				tc.action, b.Keys, tc.key)
		}
	}
}

func TestSchemeForFallsBackToMino(t *testing.T) {
	sc := SchemeFor("no-such-scheme")
	if got := sc.Binding(Save).Keys; len(got) == 0 || got[0] != "ctrl+s" {
		t.Fatalf("fallback scheme Save = %v, want ctrl+s", got)
	}
}
