package active

import (
	"context"
	"time"

	"github.com/codyconfer/sisyphus/kv"

	"github.com/codyconfer/mino/internal/signals"
	"github.com/codyconfer/mino/plugin/stream"
)

type State = stream.State

type Seen = stream.Seen

func NewState(store *kv.Store) *State {
	if store == nil {
		return stream.NewState(nil)
	}
	return stream.NewState(store)
}

func Poll(ctx context.Context, name string, interval time.Duration, step func(ctx context.Context) ([]signals.Item, error)) <-chan signals.Event {
	return stream.Poll(ctx, name, interval, step)
}

func PollAdaptive(ctx context.Context, name string, interval time.Duration, step func(ctx context.Context) ([]signals.Item, time.Duration, error)) <-chan signals.Event {
	return stream.PollAdaptive(ctx, name, interval, step)
}
