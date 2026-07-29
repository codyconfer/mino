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
