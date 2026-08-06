package gitlab

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/signals"
	"github.com/codyconfer/mino/internal/signals/active"
)

const (
	DefaultInterval = 60 * time.Second
	activityNS      = "gitlab:activity"
	cursorNS        = "gitlab"
	cursorKey       = "updated_after"
	// cursorOverlap absorbs clock skew between GitLab and this machine; the persistent
	// deduper swallows the repeats it causes.
	cursorOverlap = 90 * time.Second
)

func withJitter(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	return d + time.Duration(rand.Int64N(int64(d)/4+1))
}

type activeSignal struct {
	sig      *Signal
	rate     *RateHint
	interval time.Duration
	state    *active.State
}

// NewActive polls the same *Signal.Fetch the query path uses. A separate loop would make
// "my merge requests" in `mino gitlab query` and in `mino serve` two code paths that
// drift apart.
func NewActive(sig *Signal, interval time.Duration, state *active.State) signals.ActiveSignal {
	if interval <= 0 {
		interval = DefaultInterval
	}
	return &activeSignal{sig: sig, rate: sig.rate, interval: interval, state: state}
}

func (a *activeSignal) Name() string { return signalName }

func (a *activeSignal) LatencyFloor() time.Duration { return a.interval }

func (a *activeSignal) Stream(ctx context.Context) (<-chan signals.Event, error) {
	return active.PollAdaptive(ctx, signalName, a.interval, a.step()), nil
}

func (a *activeSignal) step() func(context.Context) ([]signals.Item, time.Duration, error) {
	seen := a.state.Seen(activityNS)
	cursor := a.state.Cursor(cursorNS, cursorKey)

	var (
		since  string
		loaded bool
		fails  int
	)
	return func(ctx context.Context) ([]signals.Item, time.Duration, error) {
		if !loaded {
			since, loaded = cursor.Load(ctx), true
		}
		a.sig.setSince(since)

		secs, err := a.sig.Fetch(ctx)
		if err != nil {
			fails++
			return nil, a.backoff(fails), err
		}
		fails = 0

		items, high := flatten(secs)
		if !high.IsZero() {
			next := high.Add(-cursorOverlap).UTC().Format(time.RFC3339)
			if err := cursor.Save(ctx, next); err != nil && !errors.Is(err, context.Canceled) {
				log.Debugf("gitlab: saving the activity cursor: %v", err)
			}
			since = next
		}
		return seen.Unseen(ctx, items, activityKey), a.nextInterval(), nil
	}
}

func (a *activeSignal) nextInterval() time.Duration {
	return max(a.interval, a.rate.delay(timeNow()))
}

func (a *activeSignal) backoff(fails int) time.Duration {
	return max(a.rate.delay(timeNow()), withJitter(backoffInterval(a.interval, fails)))
}

func flatten(secs []signals.Section) ([]signals.Item, time.Time) {
	var (
		items []signals.Item
		high  time.Time
	)
	for _, sec := range secs {
		for _, it := range sec.Items {
			items = append(items, it)
			if it.Timestamp.After(high) {
				high = it.Timestamp
			}
		}
	}
	return items, high
}

// activityKey includes the raw updated_at so every MR edit and every pipeline transition
// fires exactly once. The separator is NUL rather than a printable character because a
// project path is user-controlled text.
func activityKey(it signals.Item) string {
	return it.Kind + "\x00" + it.Meta["project"] + "\x00" + it.Meta["iid"] + "\x00" + it.Meta["updated"]
}
