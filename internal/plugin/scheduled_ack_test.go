package plugin_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/plugin"
	"github.com/codyconfer/mino/internal/plugin/ntr"
	"github.com/codyconfer/mino/internal/signals"
)

func TestRunScheduledAcksOnlyAfterOnFire(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	home := t.TempDir()
	st, err := ntr.Open(ctx, home, "r")
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateReminder(ctx, "wire", time.Now().UTC().Add(-time.Minute))
	st.Close()
	if err != nil {
		t.Fatal(err)
	}

	job := ntr.ReminderJob{Home: home, Role: "r", Now: time.Now}
	failOnce := true
	failed := make(chan struct{})
	delivered := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- plugin.RunScheduled(ctx, []plugin.Scheduled{job}, func(string, []signals.Section) error {
			if failOnce {
				failOnce = false
				close(failed)
				return errors.New("notify sink busy")
			}
			select {
			case <-delivered:
			default:
				close(delivered)
			}
			cancel()
			return nil
		})
	}()

	select {
	case <-failed:
	case <-time.After(10 * time.Second):
		t.Fatal("schedule did not reach the first onFire attempt")
	}
	st, err = ntr.Open(context.Background(), home, "r")
	if err != nil {
		t.Fatal(err)
	}
	due, err := st.DueReminders(context.Background(), time.Now().UTC())
	st.Close()
	if err != nil || len(due) != 1 {
		t.Fatalf("still due after failed onFire: %v err=%v", due, err)
	}

	select {
	case <-delivered:
	case <-time.After(10 * time.Second):
		t.Fatal("schedule did not recover after onFire success")
	}
	<-errCh

	st, err = ntr.Open(context.Background(), home, "r")
	if err != nil {
		t.Fatal(err)
	}
	due, err = st.DueReminders(context.Background(), time.Now().UTC())
	st.Close()
	if err != nil || len(due) != 0 {
		t.Fatalf("after successful onFire+Ack due=%v err=%v", due, err)
	}
}
