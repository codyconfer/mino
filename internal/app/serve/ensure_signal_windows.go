//go:build windows

package serve

import "os"

func signalStop(proc *os.Process) {
	_ = proc.Kill()
}
