package keymap

import (
	"strings"
)

// Hotkey targets for config keybinds (see config.Config.Keybinds).
const (
	TargetNoteNew   = "ntr.note.new"
	TargetTaskNew   = "ntr.task.new"
	TargetRemindNew = "ntr.remind.new"
	flightPrefix    = "flight:"
)

// NormalizeKey lowercases bubbletea key strings so alt+N matches alt+n.
func NormalizeKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

// ResolveHotkey looks up key in binds (case-insensitive keys). Empty target → miss.
func ResolveHotkey(binds map[string]string, key string) (target string, ok bool) {
	if len(binds) == 0 {
		return "", false
	}
	want := NormalizeKey(key)
	if want == "" {
		return "", false
	}
	if t, hit := binds[want]; hit {
		return strings.TrimSpace(t), t != ""
	}
	for k, t := range binds {
		if NormalizeKey(k) == want {
			t = strings.TrimSpace(t)
			return t, t != ""
		}
	}
	return "", false
}

// FlightTarget returns the flight name when target is a flight open directive.
// Accepts bare names ("morning") or "flight:morning". NTR create targets return ok=false.
func FlightTarget(target string) (name string, ok bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", false
	}
	switch target {
	case TargetNoteNew, TargetTaskNew, TargetRemindNew:
		return "", false
	}
	if strings.HasPrefix(target, flightPrefix) {
		name = strings.TrimSpace(strings.TrimPrefix(target, flightPrefix))
		return name, name != ""
	}
	return target, true
}
