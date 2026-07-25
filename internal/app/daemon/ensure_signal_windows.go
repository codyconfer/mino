//go:build windows && !nodaemon

package daemon

import "os"

func signalStop(proc *os.Process) {
	_ = proc.Kill()
}
