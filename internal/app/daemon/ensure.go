package daemon

import (
	"context"
	"os"
	"sync"
	"time"

	muninterm "github.com/codyconfer/viewkit/term"

	"github.com/codyconfer/munin/internal/log"
)

// EnsureLiveProvider dials an existing serve socket, or starts `munin serve
// <flight>` as a silent background process owned by this deck session.
//
// The returned stop func terminates a serve that this call started; it is a
// no-op when an existing provider was reused via Dial (so an installed daemon
// or a manually started serve is never killed).
//
// Ownership is enforced two ways on Unix:
//   - stop() closes a held lifeline pipe and signals the child (normal quit)
//   - the child watches that pipe and exits when the write end closes, which
//     the kernel does if this process dies unexpectedly (SIGKILL/crash/SIGHUP)
//
// On Windows, stop() kills the owned child; unexpected parent death does not
// auto-reap (no ExtraFiles / job-object wiring yet).
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
	return owned.Stop
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
