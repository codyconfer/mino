package kubectl

import (
	"context"
	"errors"
	"time"

	"github.com/codyconfer/mino/external/plugins/internal/stream"
	"github.com/codyconfer/mino/plugin"
)

type active struct {
	sig      Signal
	interval time.Duration
	seen     *stream.Seen
}

func NewActive(sig Signal, interval time.Duration, state *stream.State) plugin.Stream {
	if interval <= 0 {
		interval = DefaultInterval
	}
	return &active{sig: sig, interval: interval, seen: state.Seen(SignalName)}
}

func (a *active) Name() string { return SignalName }

func (a *active) LatencyFloor() time.Duration { return a.interval }

func (a *active) Stream(ctx context.Context) (<-chan plugin.Event, error) {
	step := func(sctx context.Context) ([]plugin.Item, error) {
		sections, err := a.sig.Fetch(sctx)
		if err != nil {
			return nil, err
		}
		var items []plugin.Item
		var errs []error
		for _, sec := range sections {
			if sec.Err != nil {
				errs = append(errs, sec.Err)
				continue
			}
			items = append(items, sec.Items...)
		}
		if len(errs) > 0 && len(errs) == len(sections) {
			return nil, errors.Join(errs...)
		}
		return a.seen.Unseen(sctx, items, itemKey), nil
	}
	return stream.Poll(ctx, SignalName, a.interval, step), nil
}

func itemKey(it plugin.Item) string {
	if it.Kind == "event" {
		return it.Kind + "\x00" + eventKey(it)
	}
	return it.Kind + "\x00" + it.Title + "\x00" + it.Meta["reason"]
}
