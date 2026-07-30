package active

import (
	"context"
	"testing"
	"time"

	"github.com/codyconfer/munin/internal/signals"
)

func TestPollStepGetsDeadline(t *testing.T) {
	seen := make(chan time.Duration, 1)
	step := func(ctx context.Context) ([]signals.Item, error) {
		dl, ok := ctx.Deadline()
		if !ok {
			seen <- 0
			return nil, nil
		}
		seen <- time.Until(dl)
		return nil, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	Poll(ctx, "test", 200*time.Millisecond, step)
	select {
	case d := <-seen:
		if d <= 0 {
			t.Fatal("poll step ran on a context with no deadline: a hung request would block the source forever")
		}
		if d > 200*time.Millisecond {
			t.Fatalf("step deadline = %s, want at most the 200ms poll interval", d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("step never ran")
	}
}

func TestPollAdaptiveStepGetsDeadline(t *testing.T) {
	seen := make(chan bool, 1)
	step := func(ctx context.Context) ([]signals.Item, time.Duration, error) {
		_, ok := ctx.Deadline()
		seen <- ok
		return nil, 0, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	PollAdaptive(ctx, "test", 200*time.Millisecond, step)
	select {
	case ok := <-seen:
		if !ok {
			t.Fatal("adaptive poll step ran on a context with no deadline")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("step never ran")
	}
}

func TestPollBlockedStepEmitsError(t *testing.T) {
	step := func(ctx context.Context) ([]signals.Item, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := Poll(ctx, "test", 200*time.Millisecond, step)
	select {
	case ev := <-events:
		if ev.Section.Err == nil {
			t.Fatalf("want an error event, got %#v", ev.Section)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a step blocked on its context never returned: the poll loop is unbounded")
	}
}

func TestStepTimeout(t *testing.T) {
	cases := []struct {
		interval time.Duration
		want     time.Duration
	}{
		{interval: 10 * time.Second, want: 10 * time.Second},
		{interval: 0, want: maxStepTimeout},
		{interval: -time.Second, want: maxStepTimeout},
		{interval: time.Hour, want: maxStepTimeout},
		{interval: time.Millisecond, want: time.Millisecond},
	}
	for _, c := range cases {
		if got := stepTimeout(c.interval); got != c.want {
			t.Errorf("stepTimeout(%s) = %s, want %s", c.interval, got, c.want)
		}
	}
}
