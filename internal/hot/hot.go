package hot

import (
	"context"
	"sync"
	"time"

	"github.com/codyconfer/munin/internal/signals"
)

func FanIn(ctx context.Context, chans ...<-chan signals.Event) <-chan signals.Event {
	out := make(chan signals.Event)
	var wg sync.WaitGroup
	for _, ch := range chans {
		wg.Add(1)
		go func(c <-chan signals.Event) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case ev, ok := <-c:
					if !ok {
						return
					}
					select {
					case out <- ev:
					case <-ctx.Done():
						return
					}
				}
			}
		}(ch)
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

func Poll(ctx context.Context, name string, interval time.Duration, step func(ctx context.Context) ([]signals.Item, error)) <-chan signals.Event {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	out := make(chan signals.Event)
	go func() {
		defer close(out)
		send := func(sec signals.Section) bool {
			select {
			case out <- signals.Event{Source: name, Section: sec, At: time.Now()}:
				return true
			case <-ctx.Done():
				return false
			}
		}
		tick := func() bool {
			items, err := step(ctx)
			if err != nil {
				return send(signals.Section{Signal: name, Title: name, Err: err})
			}
			if len(items) == 0 {
				return true
			}
			return send(signals.Section{Signal: name, Title: name, Items: items})
		}
		if !tick() {
			return
		}
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if !tick() {
					return
				}
			}
		}
	}()
	return out
}

type Seen struct {
	keys  map[string]bool
	first bool
}

func NewSeen() *Seen { return &Seen{keys: map[string]bool{}, first: true} }

func (s *Seen) Fresh(items []signals.Item, key func(signals.Item) string) []signals.Item {
	var out []signals.Item
	for _, it := range items {
		k := key(it)
		if s.keys[k] {
			continue
		}
		s.keys[k] = true
		if !s.first {
			out = append(out, it)
		}
	}
	s.first = false
	return out
}
