//go:build unix

package serve

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func detachProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func startSilent(bin string, args ...string) (*ownedServe, error) {
	lifeR, lifeW, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin, args...)
	cmd.Env = withDeckLifelineEnv(os.Environ(), 3)
	cmd.ExtraFiles = []*os.File{lifeR}
	detachProcess(cmd)
	if err := cmd.Start(); err != nil {
		_ = lifeR.Close()
		_ = lifeW.Close()
		return nil, err
	}
	_ = lifeR.Close()

	owned := &ownedServe{
		proc: cmd.Process,
		life: lifeW,
		done: make(chan struct{}),
	}
	go func() {
		_, _ = cmd.Process.Wait()
		close(owned.done)
	}()
	return owned, nil
}

func withDeckLifelineEnv(env []string, fd int) []string {
	prefix := deckLifelineEnv + "="
	out := make([]string, 0, len(env)+1)
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			continue
		}
		out = append(out, e)
	}
	return append(out, fmt.Sprintf("%s=%d", deckLifelineEnv, fd))
}
