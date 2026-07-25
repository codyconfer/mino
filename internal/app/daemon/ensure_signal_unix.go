//go:build unix

package daemon

import (
	"os"
	"syscall"
)

func signalStop(proc *os.Process) {
	_ = proc.Signal(syscall.SIGTERM)
}
