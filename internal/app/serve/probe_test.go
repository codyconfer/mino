package serve

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	sysdaemon "github.com/codyconfer/sisyphus/daemon"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/signals"
)

func dialGoroutines() int {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	return strings.Count(string(buf[:n]), "sisyphus/daemon.Dial[")
}

func TestEnsureLiveProviderProbeLeaksNoConnection(t *testing.T) {
	home := t.TempDir()
	s := &Server{App: &app.App{
		Cfg:        &config.Config{Home: home, Role: "test"},
		Directives: &config.Directives{},
	}}

	ln, err := sysdaemon.Listen(config.SocketPrefix, s.SocketPath())
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	subj := sysdaemon.NewSubject[signals.Event]()
	defer subj.Close()
	go sysdaemon.Broadcast(ctx, ln, subj, 8, Encode)
	time.Sleep(50 * time.Millisecond)

	before := dialGoroutines()

	stop := s.EnsureLiveProvider(ctx, "flight")
	defer stop()

	subj.Next(signals.Event{Source: "demo", At: time.Now(), Section: signals.Section{
		Signal: "demo",
		Items:  []signals.Item{{Title: "one"}},
	}})

	deadline := time.Now().Add(3 * time.Second)
	for {
		after := dialGoroutines()
		if after <= before {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("liveness probe leaked %d client connection goroutine(s)", after-before)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
