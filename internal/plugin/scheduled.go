package plugin

import (
	"context"
	"sync"
	"time"

	"github.com/codyconfer/sisyphus/daemon"

	"github.com/codyconfer/munin/internal/log"
	"github.com/codyconfer/munin/internal/signals"
)

const (
	scanInterval = time.Second
	nextTimeout  = 30 * time.Second
	fetchTimeout = 2 * time.Minute
	ackTimeout   = 15 * time.Second
)

type ScheduledAck interface {
	Ack(ctx context.Context, sections []signals.Section) error
}

func RunScheduled(ctx context.Context, jobs []Scheduled, onFire func(name string, sections []signals.Section) error) error {
	if onFire == nil {
		onFire = func(string, []signals.Section) error { return nil }
	}
	if len(jobs) == 0 {
		return daemon.Schedule(ctx, scanInterval)
	}
	var wg sync.WaitGroup
	for _, job := range jobs {
		j := job
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := daemon.Schedule(ctx, scanInterval, scheduleJob(j, onFire)); err != nil && ctx.Err() == nil {
				log.Warnf("scheduled %s: %v", j.Name(), err)
			}
		}()
	}
	wg.Wait()
	return nil
}

func scheduleJob(j Scheduled, onFire func(name string, sections []signals.Section) error) daemon.ScheduleJob {
	return daemon.ScheduleJob{
		Name: j.Name(),
		Next: func(ctx context.Context, now time.Time) (daemon.Due, error) {
			ctx, cancel := context.WithTimeout(ctx, nextTimeout)
			defer cancel()
			at, ready, err := j.Next(ctx, now)
			return daemon.Due{At: at, Ready: ready}, err
		},
		Run: func(ctx context.Context) error {
			secs, err := fetchOnce(ctx, j)
			if err != nil {
				return err
			}
			if err := onFire(j.Name(), secs); err != nil {
				return err
			}
			ack, ok := j.(ScheduledAck)
			if !ok {
				return nil
			}
			actx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ackTimeout)
			defer cancel()
			return ack.Ack(actx, secs)
		},
	}
}

func fetchOnce(ctx context.Context, j Scheduled) ([]signals.Section, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	return j.Fetch(ctx)
}
