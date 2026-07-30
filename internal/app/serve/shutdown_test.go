package serve

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sysdaemon "github.com/codyconfer/sisyphus/daemon"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/audit"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/signals"
)

func testServer(t *testing.T, home string) (*Server, *audit.Store) {
	t.Helper()
	st, err := audit.Open(context.Background(), filepath.Join(home, "audit.duckdb"))
	if err != nil {
		t.Fatalf("audit.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return &Server{App: &app.App{
		Cfg:        &config.Config{Home: home, Role: "test"},
		Directives: &config.Directives{},
		Audit:      st,
	}}, st
}

func oneItemEvent(i int) signals.Event {
	return signals.Event{
		Source: "demo",
		At:     time.Now(),
		Section: signals.Section{
			Signal: "demo",
			Title:  "demo",
			Items:  []signals.Item{{Kind: "note", Title: fmt.Sprintf("event-%d", i)}},
		},
	}
}

func TestWatchRecordsAllEventsBeforeRollUp(t *testing.T) {
	home := t.TempDir()
	s, st := testServer(t, home)

	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan signals.Event)
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.watch(ctx, cancel, sources{events: in, join: func() {}}, notifySink{})
	}()

	const want = 120
	for i := range want {
		select {
		case in <- oneItemEvent(i):
		case <-time.After(10 * time.Second):
			t.Fatalf("publishing event %d stalled", i)
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("watch did not return after cancel")
	}

	top, err := st.RecentEntries(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 1 || top[0].Kind != "flight" || top[0].Name != "serve" {
		t.Fatalf("RecentEntries = %+v", top)
	}
	children, err := st.Children(top[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != want {
		t.Errorf("recorded %d query runs, want %d (tail of the event stream was dropped)", len(children), want)
	}
	if top[0].ItemCount != want {
		t.Errorf("flight item_count = %d, want %d (roll-up ran before the audit drain finished)", top[0].ItemCount, want)
	}
	if top[0].Finished.IsZero() {
		t.Error("flight has no finished_at")
	}
}

func TestEndSessionOrdersTeardown(t *testing.T) {
	home := t.TempDir()
	s, st := testServer(t, home)

	ctx, cancel := context.WithCancel(context.Background())
	subj := sysdaemon.NewSubject[signals.Event]()

	var cancelled, joined, socketClosed atomic.Bool
	go func() {
		<-ctx.Done()
		cancelled.Store(true)
	}()

	audited := make(chan struct{})
	flightID := st.StartFlight("serve", "test")
	go func() {
		defer close(audited)
		time.Sleep(200 * time.Millisecond)
		now := time.Now()
		st.RecordQuery(flightID, "late", "test", now, now, []signals.Section{{
			Signal: "demo",
			Items:  []signals.Item{{Title: "a"}, {Title: "b"}, {Title: "c"}},
		}})
	}()

	s.endSession(session{
		cancel: cancel,
		src: sources{join: func() {
			if ctx.Err() == nil {
				t.Error("sources joined before the context was cancelled")
			}
			joined.Store(true)
		}},
		closeSock: func() {
			if !joined.Load() {
				t.Error("socket closed before the sources were joined")
			}
			socketClosed.Store(true)
		},
		subj:     subj,
		audited:  audited,
		flightID: flightID,
	})

	if !joined.Load() || !socketClosed.Load() {
		t.Fatalf("teardown incomplete: joined=%v socketClosed=%v", joined.Load(), socketClosed.Load())
	}
	row, ok, err := st.Entry(flightID)
	if err != nil || !ok {
		t.Fatalf("Entry(%d) ok=%v err=%v", flightID, ok, err)
	}
	if row.ItemCount != 3 {
		t.Errorf("flight item_count = %d, want 3 (roll-up ran before the audit observer drained)", row.ItemCount)
	}
	if row.Finished.IsZero() {
		t.Error("flight has no finished_at")
	}
}

func TestTrackJoinWaitsForFinalSourceWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	in := make(chan signals.Event)
	var wrote atomic.Bool
	go func() {
		defer close(in)
		<-ctx.Done()
		time.Sleep(100 * time.Millisecond)
		wrote.Store(true)
	}()

	out := track(ctx, &wg, in, nil)
	go func() {
		for range out {
		}
	}()

	cancel()
	joined := make(chan struct{})
	go func() {
		wg.Wait()
		close(joined)
	}()
	select {
	case <-joined:
	case <-time.After(10 * time.Second):
		t.Fatal("join never returned")
	}
	if !wrote.Load() {
		t.Fatal("join returned before the source finished its final cursor write")
	}
}

func TestTrackJoinDoesNotHangOnStuckSource(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	in := make(chan signals.Event)
	out := track(ctx, &wg, in, nil)
	go func() {
		for range out {
		}
	}()

	cancel()
	joined := make(chan struct{})
	go func() {
		wg.Wait()
		close(joined)
	}()
	select {
	case <-joined:
	case <-time.After(sourceDrainGrace + 5*time.Second):
		t.Fatal("join hung on a source that never closes its channel")
	}
}
