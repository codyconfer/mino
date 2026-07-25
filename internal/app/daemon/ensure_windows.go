//go:build windows && !nodaemon

package daemon

import "os/exec"

func detachProcess(cmd *exec.Cmd) {}

func startSilent(bin string, args ...string) (*ownedServe, error) {
	cmd := exec.Command(bin, args...)
	detachProcess(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	owned := &ownedServe{
		proc: cmd.Process,
		done: make(chan struct{}),
	}
	go func() {
		_, _ = cmd.Process.Wait()
		close(owned.done)
	}()
	return owned, nil
}
