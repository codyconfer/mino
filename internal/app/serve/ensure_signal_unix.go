//go:build unix

package serve

import (
	"os"
	"syscall"
)

func signalStop(proc *os.Process) {
	_ = proc.Signal(syscall.SIGTERM)
}
