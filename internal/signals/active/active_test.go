package active

import (
	"context"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/signals"
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
		if d > maxStepTimeout {
			t.Fatalf("step deadline = %s, want at most %s", d, maxStepTimeout)
		}
		if d <= 200*time.Millisecond {
			t.Fatalf("step deadline = %s, want the %s floor: a budget equal to a short poll interval "+
				"leaves no time for the request itself", d, minStepTimeout)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("step never ran")
	}
}

func TestPollShortIntervalStillLetsAStepFinish(t *testing.T) {
	const work = 300 * time.Millisecond
	step := func(ctx context.Context) ([]signals.Item, error) {
		select {
		case <-time.After(work):
			return []signals.Item{{Title: "done"}}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := Poll(ctx, "test", 100*time.Millisecond, step)
	select {
	case ev := <-events:
		if ev.Section.Err != nil {
			t.Fatalf("a %s step under a 100ms interval failed with %v: the per-step budget must not be the "+
				"poll period, or a short --interval breaks the source on every poll", work, ev.Section.Err)
		}
		if len(ev.Section.Items) != 1 {
			t.Errorf("items = %d, want 1", len(ev.Section.Items))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no event within 3s")
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
	case <-time.After(minStepTimeout + 3*time.Second):
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
		{interval: maxStepTimeout, want: maxStepTimeout},
		{interval: minStepTimeout, want: minStepTimeout},
		{interval: minStepTimeout + time.Second, want: minStepTimeout + time.Second},
		{interval: time.Second, want: minStepTimeout},
		{interval: 100 * time.Millisecond, want: minStepTimeout},
		{interval: time.Millisecond, want: minStepTimeout},
	}
	for _, c := range cases {
		if got := stepTimeout(c.interval); got != c.want {
			t.Errorf("stepTimeout(%s) = %s, want %s", c.interval, got, c.want)
		}
	}
}
