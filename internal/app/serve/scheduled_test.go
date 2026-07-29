package serve

import (
	"context"
	"testing"
	"time"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/plugin/ntr"
	"github.com/codyconfer/munin/internal/signals/active"
	"github.com/codyconfer/munin/internal/testenv"
)

func TestScheduledEventsEmitsDueReminder(t *testing.T) {
	home := t.TempDir()
	testenv.Isolate(t)
	plugin.LoadEnabled()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	st, err := ntr.Open(ctx, home, "default")
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateReminder(ctx, "wire-test", time.Now().UTC().Add(-time.Minute))
	st.Close()
	if err != nil {
		t.Fatal(err)
	}

	s := &Server{App: &app.App{
		Cfg: &config.Config{Home: home, Role: "default"},
		Directives: &config.Directives{
			Queries: map[string]config.Query{
				"ntr-list": {Name: "ntr-list", Signal: ntr.SignalName},
			},
		},
	}}
	ch := s.scheduledEvents(ctx, "ntr", []string{"ntr-list"}, active.NewState(nil))
	if ch == nil {
		t.Fatal("expected scheduled channel")
	}
	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatal("channel closed without event")
		}
		if len(ev.Section.Items) == 0 {
			t.Fatalf("empty items: %+v", ev)
		}
		if ev.Section.Items[0].Title != "wire-test" {
			t.Fatalf("title = %q", ev.Section.Items[0].Title)
		}
		cancel()
	case <-ctx.Done():
		t.Fatal("timeout waiting for reminder event")
	}
	select {
	case _, ok := <-ch:
		if ok {
			for range ch {
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("scheduled goroutine did not exit after cancel")
	}
}
