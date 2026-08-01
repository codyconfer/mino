package stream

import (
	"context"
	"time"

	"github.com/codyconfer/mino/plugin"
	pstream "github.com/codyconfer/mino/plugin/stream"
)

type State = pstream.State

type Seen = pstream.Seen

func NewState(store plugin.KV) *State { return pstream.NewState(store) }

func StateOf(bc plugin.BuildContext) *State { return pstream.StateOf(bc) }

func Poll(ctx context.Context, name string, interval time.Duration, step func(ctx context.Context) ([]plugin.Item, error)) <-chan plugin.Event {
	return pstream.Poll(ctx, name, interval, step)
}

func PollAdaptive(ctx context.Context, name string, interval time.Duration, step func(ctx context.Context) ([]plugin.Item, time.Duration, error)) <-chan plugin.Event {
	return pstream.PollAdaptive(ctx, name, interval, step)
}
