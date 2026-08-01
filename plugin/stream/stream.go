package stream

import (
	"context"
	"time"

	"github.com/codyconfer/sisyphus/daemon"

	"github.com/codyconfer/mino/plugin"
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

func Poll(ctx context.Context, name string, interval time.Duration, step func(ctx context.Context) ([]plugin.Item, error)) <-chan plugin.Event {
	src := daemon.Source[plugin.Item]{Interval: interval, StepTimeout: stepTimeout(interval), Step: step}
	return emit(ctx, name, src.Run(ctx))
}

func PollAdaptive(ctx context.Context, name string, interval time.Duration, step func(ctx context.Context) ([]plugin.Item, time.Duration, error)) <-chan plugin.Event {
	src := daemon.Source[plugin.Item]{Interval: interval, StepTimeout: stepTimeout(interval), StepAdaptive: step}
	return emit(ctx, name, src.Run(ctx))
}

func emit(ctx context.Context, name string, em <-chan daemon.Emission[plugin.Item]) <-chan plugin.Event {
	out := make(chan plugin.Event)
	go func() {
		defer close(out)
		for e := range em {
			sec := plugin.Section{Signal: name, Title: name}
			if e.Err != nil {
				sec.Err = e.Err
			} else {
				sec.Items = e.Items
			}
			select {
			case out <- plugin.Event{Source: name, Section: sec, At: time.Now()}:
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

func NewState(store daemon.KV) *State {
	return &State{kv: store}
}

func StateOf(bc plugin.BuildContext) *State {
	if kv := plugin.KVOf(bc); kv != nil {
		return NewState(kv)
	}
	return &State{}
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
	d  *daemon.Deduper[plugin.Item]
	kv daemon.KV
	ns string
}

func newSeen() *Seen { return &Seen{} }

func (s *Seen) Unseen(ctx context.Context, items []plugin.Item, key func(plugin.Item) string) []plugin.Item {
	if s.d == nil {
		if s.kv != nil {
			s.d = daemon.NewPersistentDeduper(ctx, key, s.kv, s.ns)
		} else {
			s.d = daemon.NewDeduper(key)
		}
	}
	return s.d.Unseen(ctx, items)
}
