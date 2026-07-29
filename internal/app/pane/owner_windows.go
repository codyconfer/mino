//go:build windows

package pane

import "os"

func ownerAlive(pid int) bool {
	if pid <= 0 {
		return true
	}
	_, err := os.FindProcess(pid)
	return err == nil
}
