package gitlab

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/signals"
	"github.com/codyconfer/mino/internal/signals/active"
)

func mrPage(updated ...string) string {
	rows := make([]string, 0, len(updated))
	for i, ts := range updated {
		rows = append(rows, fmt.Sprintf(`{
		  "id": %d, "iid": %d, "state": "opened", "title": "MR %d",
		  "web_url": "https://gitlab.com/acme/api/-/merge_requests/%d",
		  "updated_at": %q, "references": {"full": "acme/api!%d"}
		}`, 900+i, i+1, i+1, i+1, ts, i+1))
	}
	return "[" + strings.Join(rows, ",") + "]"
}

func pipelinePage(status, updated string) string {
	return fmt.Sprintf(`[{
	  "id": 90210, "iid": 12, "status": %q, "ref": "main",
	  "web_url": "https://gitlab.com/acme/api/-/pipelines/90210",
	  "updated_at": %q
	}]`, status, updated)
}

func newStep(t *testing.T, rec *recorder, selectors []string) func(context.Context) ([]signals.Item, time.Duration, error) {
	t.Helper()
	rate := &RateHint{}
	b := apiBackend(t, rec.handler())
	b.Rate = rate
	s, err := New(selectors, b, 30, WithRateHint(rate))
	if err != nil {
		t.Fatal(err)
	}
	a := NewActive(s, DefaultInterval, active.NewState(nil)).(*activeSignal)
	return a.step()
}

func TestStreamTreatsTheFirstPollAsBaseline(t *testing.T) {
	freezeClock(t, "2026-08-05T12:00:00Z")
	rec := newRecorder(map[string]string{
		"merge_requests": mrPage("2026-08-04T10:00:00Z", "2026-08-04T11:00:00Z"),
	})
	step := newStep(t, rec, []string{"kind:mr scope:assigned"})

	first, _, err := step(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 0 {
		t.Errorf("a cold start emitted %d items; every open merge request would ring the bell the "+
			"first time serve runs", len(first))
	}

	second, _, err := step(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 0 {
		t.Errorf("the second poll re-emitted %d unchanged items", len(second))
	}
}

func TestStreamReEmitsWhenUpdatedAtAdvances(t *testing.T) {
	freezeClock(t, "2026-08-05T12:00:00Z")
	routes := map[string]string{"merge_requests": mrPage("2026-08-04T10:00:00Z")}
	rec := newRecorder(routes)
	step := newStep(t, rec, []string{"kind:mr scope:assigned"})

	if _, _, err := step(context.Background()); err != nil {
		t.Fatal(err)
	}
	routes["merge_requests"] = mrPage("2026-08-04T11:00:00Z")

	items, _, err := step(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Errorf("an edited MR emitted %d items, want exactly 1; the dedupe key carries updated_at "+
			"so an edit is new but a re-read is not", len(items))
	}

	again, _, err := step(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Errorf("the same edit emitted twice (%d items)", len(again))
	}
}

func TestStreamEmitsEveryPipelineTransition(t *testing.T) {
	freezeClock(t, "2026-08-05T12:00:00Z")
	routes := map[string]string{
		"projects/acme%2Fapi/pipelines": pipelinePage("pending", "2026-08-05T09:00:00Z"),
	}
	rec := newRecorder(routes)
	step := newStep(t, rec, []string{"kind:pipeline project:acme/api"})

	if _, _, err := step(context.Background()); err != nil {
		t.Fatal(err)
	}

	emitted := 0
	for i, stage := range []struct{ status, updated string }{
		{"pending", "2026-08-05T09:00:30Z"},
		{"running", "2026-08-05T09:01:00Z"},
		{"success", "2026-08-05T09:05:00Z"},
	} {
		routes["projects/acme%2Fapi/pipelines"] = pipelinePage(stage.status, stage.updated)
		items, _, err := step(context.Background())
		if err != nil {
			t.Fatalf("poll %d: %v", i, err)
		}
		emitted += len(items)
	}
	if emitted != 3 {
		t.Errorf("a pipeline moving pending -> running -> success emitted %d events, want 3; watching "+
			"CI is the point of streaming pipelines", emitted)
	}
}

func TestStreamSendsTheCursorOnTheSecondPoll(t *testing.T) {
	freezeClock(t, "2026-08-05T12:00:00Z")
	rec := newRecorder(map[string]string{"merge_requests": mrPage("2026-08-04T10:00:00Z")})
	step := newStep(t, rec, []string{"kind:mr scope:assigned"})

	if _, _, err := step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := rec.seen()[0]; strings.Contains(got, "updated_after") {
		t.Errorf("the first poll carried a cursor: %q", got)
	}

	if _, _, err := step(context.Background()); err != nil {
		t.Fatal(err)
	}
	second := rec.seen()[1]
	if !strings.Contains(second, "updated_after") {
		t.Fatalf("the second poll carried no cursor: %q; without one every tick refetches "+
			"everything open", second)
	}
	// The high-water mark was 10:00:00 and the overlap is 90s, so the cursor sits before it.
	if !strings.Contains(second, "2026-08-04T09%3A58%3A30Z") {
		t.Errorf("cursor = %q, want the watermark minus the %v skew overlap", second, cursorOverlap)
	}
}

func TestStreamDoesNotAdvanceTheCursorOnFailure(t *testing.T) {
	freezeClock(t, "2026-08-05T12:00:00Z")
	rec := newRecorder(map[string]string{"merge_requests": mrPage("2026-08-04T10:00:00Z")})
	step := newStep(t, rec, []string{"kind:mr scope:assigned"})

	if _, _, err := step(context.Background()); err != nil {
		t.Fatal(err)
	}
	rec.status["merge_requests"] = http.StatusBadGateway
	if _, _, err := step(context.Background()); err == nil {
		t.Fatal("a 502 was swallowed")
	}
	delete(rec.status, "merge_requests")

	if _, _, err := step(context.Background()); err != nil {
		t.Fatal(err)
	}
	last := rec.seen()[len(rec.seen())-1]
	if !strings.Contains(last, "2026-08-04T09%3A58%3A30Z") {
		t.Errorf("cursor = %q after a failed poll, want the last good watermark; advancing past a "+
			"failure would skip whatever changed during it", last)
	}
}

func TestStreamBacksOffAndRecovers(t *testing.T) {
	freezeClock(t, "2026-08-05T12:00:00Z")
	rec := newRecorder(map[string]string{"merge_requests": mrPage("2026-08-04T10:00:00Z")})
	rec.status["merge_requests"] = http.StatusBadGateway
	step := newStep(t, rec, []string{"kind:mr scope:assigned"})

	_, first, err := step(context.Background())
	if err == nil {
		t.Fatal("a 502 was swallowed")
	}
	_, second, err := step(context.Background())
	if err == nil {
		t.Fatal("a 502 was swallowed")
	}
	if second <= first {
		t.Errorf("interval went %v then %v, want it to grow while the instance is unhealthy",
			first, second)
	}

	delete(rec.status, "merge_requests")
	_, recovered, err := step(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if recovered != DefaultInterval {
		t.Errorf("interval = %v after recovery, want the base %v", recovered, DefaultInterval)
	}
}

func TestStreamHonoursRetryAfter(t *testing.T) {
	freezeClock(t, "2026-08-05T12:00:00Z")
	rec := newRecorder(map[string]string{})
	rec.status["merge_requests"] = http.StatusTooManyRequests
	rec.headers = map[string]string{"Retry-After": "300"}
	step := newStep(t, rec, []string{"kind:mr scope:assigned"})

	_, wait, err := step(context.Background())
	if err == nil {
		t.Fatal("a 429 was swallowed")
	}
	if wait < 300*time.Second {
		t.Errorf("next poll in %v, want at least the 300s the instance asked for", wait)
	}
}

func TestActivityKeySeparatesUserControlledText(t *testing.T) {
	a := signals.Item{Kind: "mr", Meta: map[string]string{"project": "acme/api", "iid": "1", "updated": "t"}}
	b := signals.Item{Kind: "mr", Meta: map[string]string{"project": "acme", "iid": "api|1", "updated": "t"}}
	if activityKey(a) == activityKey(b) {
		t.Error("two different items share a dedupe key; a project path is user-controlled text, so " +
			"the separator has to be one it cannot contain")
	}

	mr := signals.Item{Kind: "mr", Meta: map[string]string{"project": "p", "iid": "1", "updated": "t"}}
	issue := signals.Item{Kind: "issue", Meta: map[string]string{"project": "p", "iid": "1", "updated": "t"}}
	if activityKey(mr) == activityKey(issue) {
		t.Error("MR !1 and issue #1 in the same project share a dedupe key")
	}
}

func TestLatencyFloorReportsTheInterval(t *testing.T) {
	s, err := New([]string{"kind:mr scope:assigned"}, nil, 30)
	if err != nil {
		t.Fatal(err)
	}
	a := NewActive(s, 30*time.Second, active.NewState(nil))
	if a.LatencyFloor() != 30*time.Second {
		t.Errorf("LatencyFloor = %v, want the configured interval", a.LatencyFloor())
	}
	if a.Name() != signalName {
		t.Errorf("Name = %q", a.Name())
	}
	if got := NewActive(s, 0, active.NewState(nil)).LatencyFloor(); got != DefaultInterval {
		t.Errorf("zero interval = %v, want the %v default", got, DefaultInterval)
	}
}
