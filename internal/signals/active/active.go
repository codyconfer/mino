package active

import (
	"context"
	"time"

	"github.com/codyconfer/sisyphus/daemon"
	"github.com/codyconfer/sisyphus/kv"

	"github.com/codyconfer/munin/internal/signals"
)

const (
	minStepTimeout = 5 * time.Second
	maxStepTimeout = 2 * time.Minute
)

func stepTimeout(interval time.Duration) time.Duration {
	switch {
	case interval <= 0 || interval > maxStepTimeout:
		return maxStepTimeout
	case interval < minStepTimeout:
		return minStepTimeout
	default:
		return interval
	}
}

func Poll(ctx context.Context, name string, interval time.Duration, step func(ctx context.Context) ([]signals.Item, error)) <-chan signals.Event {
	timeout := stepTimeout(interval)
	bounded := func(ctx context.Context) ([]signals.Item, error) {
		sctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return step(sctx)
	}
	em := daemon.Poll(ctx, interval, bounded)
	return emit(ctx, name, em)
}

func PollAdaptive(ctx context.Context, name string, interval time.Duration, step func(ctx context.Context) ([]signals.Item, time.Duration, error)) <-chan signals.Event {
	timeout := stepTimeout(interval)
	bounded := func(ctx context.Context) ([]signals.Item, time.Duration, error) {
		sctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return step(sctx)
	}
	em := daemon.PollAdaptive(ctx, interval, bounded)
	return emit(ctx, name, em)
}

func emit(ctx context.Context, name string, em <-chan daemon.Emission[signals.Item]) <-chan signals.Event {
	out := make(chan signals.Event)
	go func() {
		defer close(out)
		for e := range em {
			sec := signals.Section{Signal: name, Title: name}
			if e.Err != nil {
				sec.Err = e.Err
			} else {
				sec.Items = e.Items
			}
			select {
			case out <- signals.Event{Source: name, Section: sec, At: time.Now()}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

type State struct {
	kv daemon.KV
}

func NewState(store *kv.Store) *State {
	if store == nil {
		return &State{}
	}
	return &State{kv: store}
}

func (s *State) KV() daemon.KV {
	if s == nil {
		return nil
	}
	return s.kv
}

func (s *State) Cursor(namespace, key string) *daemon.Cursor {
	if s == nil {
		return nil
	}
	return daemon.NewCursor(s.kv, namespace, key)
}

func (s *State) Seen(namespace string) *Seen {
	if s == nil {
		return newSeen()
	}
	return &Seen{kv: s.kv, ns: namespace}
}

type Seen struct {
	d  *daemon.Deduper[signals.Item]
	kv daemon.KV
	ns string
}

func newSeen() *Seen { return &Seen{} }

func (s *Seen) Fresh(ctx context.Context, items []signals.Item, key func(signals.Item) string) []signals.Item {
	if s.d == nil {
		if s.kv != nil {
			s.d = daemon.NewPersistentDeduper(ctx, key, s.kv, s.ns)
		} else {
			s.d = daemon.NewDeduper(key)
		}
	}
	return s.d.Fresh(ctx, items)
}
