//go:build !nodaemon

package daemon

import (
	"context"
	"os"
	"sync"
	"time"

	sysdaemon "github.com/codyconfer/sisyphus/daemon"
	muninterm "github.com/codyconfer/viewkit/term"

	"github.com/codyconfer/munin/internal/log"
)

func (s *Server) EnsureLiveProvider(ctx context.Context, flight string, selfArgs ...string) (stop func()) {
	stop = func() {}
	if _, ok := s.Dial(ctx); ok {
		return stop
	}
	self, err := muninterm.Self()
	if err != nil {
		log.Debugf("deck: cannot locate munin binary to start a serve provider: %v", err)
		return stop
	}
	args := append([]string{"serve", flight}, selfArgs...)
	owned, err := startSilent(self, args...)
	if err != nil {
		log.Debugf("deck: could not start a serve provider: %v", err)
		return stop
	}
	waitListening(s.SocketPath(), 2*time.Second)
	return owned.Stop
}

func waitListening(path string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if sysdaemon.IsListening(pipePrefix, path) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	log.Debugf("deck: serve socket %s not listening within %s", path, timeout)
}

type ownedServe struct {
	proc *os.Process
	life *os.File
	done chan struct{}
	once sync.Once
}

func (o *ownedServe) Stop() {
	if o == nil {
		return
	}
	o.once.Do(func() {
		if o.life != nil {
			_ = o.life.Close()
			o.life = nil
		}
		if o.proc != nil {
			signalStop(o.proc)
		}
		select {
		case <-o.done:
		case <-time.After(2 * time.Second):
			if o.proc != nil {
				_ = o.proc.Kill()
			}
			<-o.done
		}
	})
}
