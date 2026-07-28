package keymap

import (
	"strings"
)

const (
	TargetNoteNew   = "ntr.note.new"
	TargetTaskNew   = "ntr.task.new"
	TargetRemindNew = "ntr.remind.new"
	TargetRoleNext  = "role.next"
	TargetRolePrev  = "role.prev"
	flightPrefix    = "flight:"
)

func NormalizeKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

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

func FlightTarget(target string) (name string, ok bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", false
	}
	switch target {
	case TargetNoteNew, TargetTaskNew, TargetRemindNew, TargetRoleNext, TargetRolePrev:
		return "", false
	}
	if strings.HasPrefix(target, flightPrefix) {
		name = strings.TrimSpace(strings.TrimPrefix(target, flightPrefix))
		return name, name != ""
	}
	return target, true
}
