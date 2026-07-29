package pane

import (
	"os"
	"strconv"
)

const OwnerEnv = "MUNIN_PANE_OWNER"

func OwnerPID() int {
	pid, err := strconv.Atoi(os.Getenv(OwnerEnv))
	if err != nil {
		return 0
	}
	return pid
}

func OwnerWatch() func() bool {
	pid := OwnerPID()
	if pid <= 0 {
		return nil
	}
	return func() bool { return ownerAlive(pid) }
}
