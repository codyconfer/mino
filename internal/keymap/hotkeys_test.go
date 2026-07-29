package keymap

import (
	"testing"

	"github.com/codyconfer/viewkit/keys"
)

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
		got, ok := keys.Resolve(binds, tc.key)
		if !ok || got != tc.want {
			t.Errorf("keys.Resolve(%q) = %q,%v want %q,true", tc.key, got, ok, tc.want)
		}
	}
	if _, ok := keys.Resolve(binds, "n"); ok {
		t.Error("plain n must not resolve (reserved for in-list new)")
	}
	if _, ok := keys.Resolve(nil, "alt+n"); ok {
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
	if _, ok := FlightTarget(TargetRoleNext); ok {
		t.Error("role.next must not look like a flight")
	}
	if _, ok := FlightTarget("flight:"); ok {
		t.Error("empty flight name")
	}
}

func TestResolveHotkeyRoleCycle(t *testing.T) {
	binds := map[string]string{
		"alt+[": TargetRolePrev,
		"alt+]": TargetRoleNext,
	}
	if got, ok := keys.Resolve(binds, "alt+["); !ok || got != TargetRolePrev {
		t.Errorf("alt+[ = %q,%v", got, ok)
	}
	if got, ok := keys.Resolve(binds, "alt+]"); !ok || got != TargetRoleNext {
		t.Errorf("alt+] = %q,%v", got, ok)
	}
}
