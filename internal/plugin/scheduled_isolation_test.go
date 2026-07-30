package plugin_test

import (
	"context"
	"testing"
	"time"

	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/signals"
)

type blockingJob struct {
	name    string
	entered chan struct{}
}

func (b *blockingJob) Name() string { return b.name }

func (b *blockingJob) Next(ctx context.Context, _ time.Time) (time.Time, bool, error) {
	select {
	case b.entered <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return time.Time{}, false, ctx.Err()
}

func (b *blockingJob) Fetch(ctx context.Context) ([]signals.Section, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

type readyJob struct {
	name  string
	fired chan string
}

func (r *readyJob) Name() string { return r.name }

func (r *readyJob) Next(context.Context, time.Time) (time.Time, bool, error) {
	return time.Time{}, true, nil
}

func (r *readyJob) Fetch(context.Context) ([]signals.Section, error) {
	return []signals.Section{{Signal: r.name, Title: "t"}}, nil
}

func TestRunScheduledIsolatesBlockingJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	blocker := &blockingJob{name: "blocker", entered: make(chan struct{}, 1)}
	good := &readyJob{name: "good", fired: make(chan string, 4)}

	done := make(chan error, 1)
	go func() {
		done <- plugin.RunScheduled(ctx, []plugin.Scheduled{blocker, good},
			func(name string, _ []signals.Section) error {
				select {
				case good.fired <- name:
				default:
				}
				return nil
			})
	}()

	select {
	case <-blocker.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("blocking job never got scheduled")
	}

	select {
	case name := <-good.fired:
		if name != "good" {
			t.Fatalf("fired %q, want good", name)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("a job blocked in Next stalled every other scheduled job")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunScheduled did not return after cancel")
	}
}

type ackProbe struct {
	acked    chan ackObs
	onFireHK func()
}

type ackObs struct {
	err         error
	hasDeadline bool
	within      bool
}

func (a *ackProbe) Name() string { return "ackprobe" }

func (a *ackProbe) Next(context.Context, time.Time) (time.Time, bool, error) {
	return time.Time{}, true, nil
}

func (a *ackProbe) Fetch(context.Context) ([]signals.Section, error) {
	return []signals.Section{{Signal: "ackprobe", Title: "t"}}, nil
}

func (a *ackProbe) Ack(ctx context.Context, _ []signals.Section) error {
	dl, ok := ctx.Deadline()
	obs := ackObs{err: ctx.Err(), hasDeadline: ok}
	if ok {
		obs.within = time.Until(dl) <= 5*time.Minute
	}
	select {
	case a.acked <- obs:
	default:
	}
	return nil
}

func TestRunScheduledAckIsBoundedNotUncancellable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	probe := &ackProbe{acked: make(chan ackObs, 1)}
	done := make(chan error, 1)
	go func() {
		done <- plugin.RunScheduled(ctx, []plugin.Scheduled{probe},
			func(string, []signals.Section) error {
				cancel()
				return nil
			})
	}()

	select {
	case obs := <-probe.acked:
		if obs.err != nil {
			t.Fatalf("Ack ctx already canceled: %v", obs.err)
		}
		if !obs.hasDeadline {
			t.Fatal("Ack ran with an unbounded context: shutdown cannot make progress and the watermark write can be SIGKILLed mid-write")
		}
		if !obs.within {
			t.Fatal("Ack deadline is not bounded to the ack timeout")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Ack never ran")
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunScheduled did not return after cancel")
	}
}
