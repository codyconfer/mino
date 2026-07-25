package keymap

import "testing"

func TestResolveHotkeyDemoDefaults(t *testing.T) {
	binds := map[string]string{
		"alt+n": "ntr.note.new",
		"alt+r": "ntr.remind.new",
		"alt+t": "ntr.task.new",
	}
	cases := []struct {
		key, want string
	}{
		{"alt+n", TargetNoteNew},
		{"alt+N", TargetNoteNew},
		{"alt+r", TargetRemindNew},
		{"alt+t", TargetTaskNew},
	}
	for _, tc := range cases {
		got, ok := ResolveHotkey(binds, tc.key)
		if !ok || got != tc.want {
			t.Errorf("ResolveHotkey(%q) = %q,%v want %q,true", tc.key, got, ok, tc.want)
		}
	}
	if _, ok := ResolveHotkey(binds, "n"); ok {
		t.Error("plain n must not resolve (reserved for in-list new)")
	}
	if _, ok := ResolveHotkey(nil, "alt+n"); ok {
		t.Error("nil binds")
	}
}

func TestFlightTarget(t *testing.T) {
	if name, ok := FlightTarget("morning"); !ok || name != "morning" {
		t.Errorf("bare flight = %q,%v", name, ok)
	}
	if name, ok := FlightTarget("flight:demo"); !ok || name != "demo" {
		t.Errorf("prefixed = %q,%v", name, ok)
	}
	if _, ok := FlightTarget(TargetNoteNew); ok {
		t.Error("ntr target must not look like a flight")
	}
	if _, ok := FlightTarget("flight:"); ok {
		t.Error("empty flight name")
	}
}
