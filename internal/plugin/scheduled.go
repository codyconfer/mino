package plugin

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/codyconfer/sisyphus/schedule"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/signals"
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
	if err := checkJobs(jobs); err != nil {
		return err
	}
	if len(jobs) == 0 {
		return schedule.Run(ctx, scanInterval)
	}
	var wg sync.WaitGroup
	failures := make([]error, len(jobs))
	for i, job := range jobs {
		i, j := i, job
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := schedule.Run(ctx, scanInterval, scheduleJob(j, onFire)); err != nil && ctx.Err() == nil {
				failures[i] = errs.Wrapf(errs.KindInternal, err, "scheduled %s", j.Name())
			}
		}()
	}
	wg.Wait()
	return errors.Join(failures...)
}

// checkJobs rejects a job list the scheduler cannot run. Without this a nil
// entry nil-derefs inside the scheduler goroutine and takes the process down.
func checkJobs(jobs []Scheduled) error {
	for i, j := range jobs {
		if j == nil {
			return errs.Newf(errs.KindInternal, "scheduled job %d is nil", i)
		}
		if strings.TrimSpace(j.Name()) == "" {
			return errs.Newf(errs.KindInternal, "scheduled job %d has no name", i)
		}
	}
	return nil
}

func scheduleJob(j Scheduled, onFire func(name string, sections []signals.Section) error) schedule.Job {
	return schedule.Job{
		Name: j.Name(),
		OnError: func(err error, fails int, retryIn time.Duration) {
			log.Warnf("scheduled %s failed (%d consecutive): %v (retry in %s)", j.Name(), fails, err, retryIn)
		},
		Next: func(ctx context.Context, now time.Time) (schedule.Due, error) {
			ctx, cancel := context.WithTimeout(ctx, nextTimeout)
			defer cancel()
			at, err := j.Next(ctx, now)
			return schedule.Due{At: at}, err
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
