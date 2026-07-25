//go:build windows

package daemon

import "os"

func signalStop(proc *os.Process) {
	_ = proc.Kill()
}
