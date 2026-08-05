package serve

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/sisyphus/ipc"
	"github.com/codyconfer/sisyphus/stream"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/signals"
)

func dialGoroutines() int {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	return strings.Count(string(buf[:n]), "sisyphus/ipc.Dial[")
}

func TestEnsureLiveProviderProbeLeaksNoConnection(t *testing.T) {
	home := shortHome(t)
	s := &Server{App: &app.App{
		Cfg:        &config.Config{Home: home, DefaultRole: "test"},
		Directives: &config.Directives{},
	}}
	t.Cleanup(s.CloseDBs)

	ln, err := ipc.Listen(config.SocketPrefix, s.SocketPath())
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	subj := stream.NewSubject[signals.Event]()
	defer subj.Close()
	go ipc.Broadcast(ctx, ln, subj, 8, Encode)
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
