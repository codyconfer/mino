//go:build !windows

package pane

import "syscall"

func ownerAlive(pid int) bool {
	if pid <= 0 {
		return true
	}
	return syscall.Kill(pid, 0) == nil
}
