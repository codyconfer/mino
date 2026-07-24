package plugin

import (
	"context"
	"time"

	"github.com/codyconfer/sisyphus/daemon"

	"github.com/codyconfer/munin/internal/signals"
)

// ScheduledAck is the optional second phase after Fetch: mark work complete
// only once the host notify path has accepted the event (ADR-10).
type ScheduledAck interface {
	Ack(ctx context.Context, sections []signals.Section) error
}

// RunScheduled drives Scheduled plugins via sisyphus daemon.Schedule until ctx
// cancels. onFire receives sections for the notify pipeline (ADR-10).
// When a job implements ScheduledAck, Ack runs only after onFire succeeds.
func RunScheduled(ctx context.Context, jobs []Scheduled, onFire func(name string, sections []signals.Section) error) error {
	if onFire == nil {
		onFire = func(string, []signals.Section) error { return nil }
	}
	var sj []daemon.ScheduleJob
	for _, job := range jobs {
		j := job
		sj = append(sj, daemon.ScheduleJob{
			Next: func(ctx context.Context, now time.Time) (daemon.Due, error) {
				at, ready, err := j.Next(ctx, now)
				return daemon.Due{At: at, Ready: ready}, err
			},
			Run: func(ctx context.Context) error {
				secs, err := j.Fetch(ctx)
				if err != nil {
					return err
				}
				if err := onFire(j.Name(), secs); err != nil {
					return err
				}
				if ack, ok := j.(ScheduledAck); ok {
					// Notify already accepted — finish ack even if schedule ctx canceled.
					return ack.Ack(context.WithoutCancel(ctx), secs)
				}
				return nil
			},
		})
	}
	return daemon.Schedule(ctx, time.Second, sj...)
}
