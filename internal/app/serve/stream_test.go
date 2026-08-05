package serve

import (
	"context"
	"testing"
	"time"

	"github.com/codyconfer/sisyphus/ipc"
	"github.com/codyconfer/sisyphus/stream"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/signals"
)

type fakeStream struct {
	name    string
	block   bool
	ch      chan signals.Event
	entered chan struct{}
	stopped chan struct{}
}

func newFakeStream(name string, block bool) *fakeStream {
	return &fakeStream{
		name:    name,
		block:   block,
		ch:      make(chan signals.Event),
		entered: make(chan struct{}),
		stopped: make(chan struct{}),
	}
}

func (f *fakeStream) Name() string { return f.name }

func (f *fakeStream) LatencyFloor() time.Duration { return time.Second }

func (f *fakeStream) Stream(ctx context.Context) (<-chan signals.Event, error) {
	close(f.entered)
	if !f.block {
		return f.ch, nil
	}
	<-ctx.Done()
	close(f.stopped)
	return nil, ctx.Err()
}

func TestOpenStreamsSkipsBlockingPlugin(t *testing.T) {
	home := t.TempDir()
	s := &Server{App: &app.App{
		Cfg:        &config.Config{Home: home, DefaultRole: "test", Timeout: "200ms"},
		Directives: &config.Directives{},
	}}
	t.Cleanup(s.CloseDBs)

	blocker := newFakeStream("blocker", true)
	good := newFakeStream("good", false)
	queries := []activeQuery{{label: "blocker", src: blocker}, {label: "good", src: good}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	res := make(chan []openStream, 1)
	go func() { res <- s.openStreams(ctx, queries) }()

	var opened []openStream
	select {
	case opened = <-res:
	case <-time.After(10 * time.Second):
		t.Fatal("openStreams blocked on a plugin whose Stream never returns")
	}
	if len(opened) != 1 || opened[0].q.label != "good" {
		t.Fatalf("opened %d streams: %+v, want only the good one", len(opened), opened)
	}
	select {
	case <-blocker.stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("the timed-out stream was never told to stop")
	}
	for _, o := range opened {
		if o.stop != nil {
			o.stop()
		}
	}
}

func TestSocketOpensDespiteBlockingPlugin(t *testing.T) {
	home := shortHome(t)
	s := &Server{App: &app.App{
		Cfg:        &config.Config{Home: home, DefaultRole: "test", Timeout: "200ms"},
		Directives: &config.Directives{},
	}}
	t.Cleanup(s.CloseDBs)

	blocker := newFakeStream("blocker", true)
	good := newFakeStream("good", false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listening := make(chan bool, 1)
	go func() {
		s.openStreams(ctx, []activeQuery{{label: "blocker", src: blocker}, {label: "good", src: good}})
		subj := stream.NewSubject[signals.Event]()
		defer subj.Close()
		closeSock := s.socket(ctx, subj)
		defer closeSock()
		listening <- ipc.IsListening(config.SocketPrefix, s.SocketPath())
	}()

	select {
	case ok := <-listening:
		if !ok {
			t.Fatalf("control socket %s is not listening", s.SocketPath())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("startup never reached the control socket: a blocking plugin stalled it")
	}
}
