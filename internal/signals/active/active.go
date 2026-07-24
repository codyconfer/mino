package active

import (
	"context"
	"time"

	"github.com/codyconfer/sisyphus/daemon"
	"github.com/codyconfer/sisyphus/kv"

	"github.com/codyconfer/munin/internal/signals"
)

func Poll(ctx context.Context, name string, interval time.Duration, step func(ctx context.Context) ([]signals.Item, error)) <-chan signals.Event {
	em := daemon.Poll(ctx, interval, step)
	return emit(ctx, name, em)
}

func PollAdaptive(ctx context.Context, name string, interval time.Duration, step func(ctx context.Context) ([]signals.Item, time.Duration, error)) <-chan signals.Event {
	em := daemon.PollAdaptive(ctx, interval, step)
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
	return &State{kv: kvAdapter{store}}
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

type kvAdapter struct{ s *kv.Store }

func (a kvAdapter) Get(namespace, key string) (string, bool, error) {
	e, ok, err := a.s.Get(namespace, key)
	return e.Value, ok, err
}

func (a kvAdapter) Set(namespace, key, value string) error {
	return a.s.Put(namespace, key, value, time.Time{})
}

func (a kvAdapter) Delete(namespace, key string) error {
	return a.s.Delete(namespace, key)
}

func (a kvAdapter) List(namespace string) (map[string]string, error) {
	m, err := a.s.List(namespace)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(m))
	for k, e := range m {
		out[k] = e.Value
	}
	return out, nil
}

type Seen struct {
	d  *daemon.Deduper[signals.Item]
	kv daemon.KV
	ns string
}

func newSeen() *Seen { return &Seen{} }

func (s *Seen) Fresh(items []signals.Item, key func(signals.Item) string) []signals.Item {
	if s.d == nil {
		if s.kv != nil {
			s.d = daemon.NewPersistentDeduper(key, s.kv, s.ns)
		} else {
			s.d = daemon.NewDeduper(key)
		}
	}
	return s.d.Fresh(items)
}
