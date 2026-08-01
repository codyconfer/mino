package audit

import (
	"context"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/signals"
)

func cancelled() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestWritesHonorACancelledContext(t *testing.T) {
	s := openTemp(t)
	ctx := cancelled()
	now := time.Now()
	secs := []signals.Section{{Signal: "github", Title: "incidents", Items: []signals.Item{{Title: "pr"}}}}

	if id := s.StartFlightContext(ctx, "aborted", "oncall"); id != 0 {
		t.Errorf("StartFlightContext with a cancelled context = %d, want 0", id)
	}
	s.RecordQueryContext(ctx, 0, "aborted", "oncall", now, time.Now(), secs)
	s.RecordActionContext(ctx, "aborted", "oncall", now, time.Now(), secs)
	s.FinishFlightContext(ctx, 1)
	if err := s.DeleteContext(ctx, 1); err == nil {
		t.Error("DeleteContext with a cancelled context = nil, want an error")
	}

	top, err := s.RecentEntries(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 0 {
		t.Errorf("cancelled writes landed anyway: %+v", top)
	}
}

func TestWritesStillLandWithALiveContext(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	now := time.Now()
	secs := []signals.Section{{Signal: "github", Title: "incidents", Items: []signals.Item{{Title: "pr"}}}}

	id := s.StartFlightContext(ctx, "triage", "oncall")
	if id == 0 {
		t.Fatal("StartFlightContext returned 0")
	}
	s.RecordQueryContext(ctx, id, "incidents", "oncall", now, time.Now(), secs)
	s.FinishFlightContext(ctx, id)

	top, err := s.RecentEntries(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 1 || top[0].Name != "triage" || top[0].Finished.IsZero() {
		t.Fatalf("RecentEntries = %+v", top)
	}
	if top[0].ItemCount != 1 {
		t.Errorf("item_count = %d, want 1 rolled up from the child", top[0].ItemCount)
	}
}

func TestNilStoreIgnoresContextVariants(t *testing.T) {
	var s *Store
	ctx := context.Background()
	if id := s.StartFlightContext(ctx, "x", ""); id != 0 {
		t.Errorf("nil StartFlightContext = %d, want 0", id)
	}
	s.RecordQueryContext(ctx, 0, "x", "", time.Now(), time.Now(), nil)
	s.RecordActionContext(ctx, "x", "", time.Now(), time.Now(), nil)
	s.FinishFlightContext(ctx, 1)
	if err := s.DeleteContext(ctx, 1); err == nil {
		t.Error("nil DeleteContext = nil, want an error")
	}
}
