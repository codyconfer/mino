package keymap

import (
	"strings"
)

const (
	TargetNoteNew   = "ntr.note.new"
	TargetTaskNew   = "ntr.task.new"
	TargetRemindNew = "ntr.remind.new"
	TargetBuckets   = "ntr.buckets"
	TargetRoleNext  = "role.next"
	TargetRolePrev  = "role.prev"
	TargetPaneInbox = "pane.inbox"
	TargetPanePop   = "pane.pop"
	TargetPaneShell = "pane.shell"
	TargetPaneClose = "pane.close"
	flightPrefix    = "flight:"
)

func FlightTarget(target string) (name string, ok bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", false
	}
	switch target {
	case TargetNoteNew, TargetTaskNew, TargetRemindNew, TargetBuckets, TargetRoleNext, TargetRolePrev,
		TargetPaneInbox, TargetPanePop, TargetPaneShell, TargetPaneClose:
		return "", false
	}
	if strings.HasPrefix(target, flightPrefix) {
		name = strings.TrimSpace(strings.TrimPrefix(target, flightPrefix))
		return name, name != ""
	}
	return target, true
}
