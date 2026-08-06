package argocd

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/codyconfer/mino/external/plugins/internal/httpx"
	"github.com/codyconfer/mino/external/plugins/internal/stream"
	"github.com/codyconfer/mino/plugin"
)

type poller struct {
	seen *stream.Seen
}

func newPollerPastItsSilentBaselinePass(t *testing.T, baseline ...plugin.Item) *poller {
	t.Helper()
	p := &poller{seen: stream.StateOf(nil).Seen(seenNamespace)}
	if emitted := p.round(t, baseline); len(emitted) != 0 {
		t.Fatalf("the baseline pass emitted %d items; a fresh deduper must announce nothing, or every "+
			"daemon restart would re-notify every application that already exists", len(emitted))
	}
	return p
}

func (p *poller) round(t *testing.T, items []plugin.Item) []plugin.Item {
	t.Helper()
	return p.seen.Unseen(context.Background(), items, appKey)
}

func item(app, revision, state string) plugin.Item {
	return plugin.Item{
		Title: app,
		Meta: map[string]string{
			"app":           app,
			"app_namespace": "argocd",
			"revision":      revision,
			"state":         state,
		},
	}
}

func TestUnchangedApplicationsStaySilent(t *testing.T) {
	synced := item("payments-api", "abc123", stateSynced)
	p := newPollerPastItsSilentBaselinePass(t, synced)

	if got := p.round(t, []plugin.Item{synced}); len(got) != 0 {
		t.Errorf("an unchanged application re-emitted %d items; the deck would notify on every tick",
			len(got))
	}
}

func TestNewApplicationEmits(t *testing.T) {
	p := newPollerPastItsSilentBaselinePass(t, item("payments-api", "abc123", stateSynced))

	got := p.round(t, []plugin.Item{
		item("payments-api", "abc123", stateSynced),
		item("checkout-web", "zzz999", stateSynced),
	})
	if len(got) != 1 || got[0].Title != "checkout-web" {
		t.Errorf("got %d items %v, want only the newly-appeared application", len(got), got)
	}
}

func TestNewRevisionReEmits(t *testing.T) {
	p := newPollerPastItsSilentBaselinePass(t, item("payments-api", "abc123", stateSynced))

	got := p.round(t, []plugin.Item{item("payments-api", "def456", stateSynced)})
	if len(got) != 1 {
		t.Errorf("a redeploy emitted %d items, want 1", len(got))
	}
}

func TestStateTransitionReEmitsWithoutARedeploy(t *testing.T) {
	p := newPollerPastItsSilentBaselinePass(t, item("payments-api", "abc123", stateSynced))

	got := p.round(t, []plugin.Item{item("payments-api", "abc123", stateDegraded)})
	if len(got) != 1 {
		t.Fatalf("synced→degraded on the same revision emitted %d items, want 1; keying only on revision "+
			"would miss a degradation with no redeploy, which is the alert an on-call engineer most needs",
			len(got))
	}
	if got[0].Meta["state"] != stateDegraded {
		t.Errorf("emitted state = %q, want %q", got[0].Meta["state"], stateDegraded)
	}
}

func TestProgressingToSyncedReEmits(t *testing.T) {
	p := newPollerPastItsSilentBaselinePass(t, item("payments-api", "abc123", stateProgressing))

	got := p.round(t, []plugin.Item{item("payments-api", "abc123", stateSynced)})
	if len(got) != 1 {
		t.Errorf("progressing→synced emitted %d items, want 1; a finished deploy is worth reporting", len(got))
	}
}

func TestAppKeyDistinguishesNamespaces(t *testing.T) {
	a := appKey(plugin.Item{Meta: map[string]string{"app": "web", "app_namespace": "team-a", "revision": "r", "state": "synced"}})
	b := appKey(plugin.Item{Meta: map[string]string{"app": "web", "app_namespace": "team-b", "revision": "r", "state": "synced"}})
	if a == b {
		t.Error("two same-named apps in different namespaces share a dedupe key; one would silence the other")
	}
}

func TestStreamEmitsMappedItems(t *testing.T) {
	var polls atomic.Int32
	emptyOnTheSilentBaselinePoll := func(w http.ResponseWriter, _ *http.Request) {
		if polls.Add(1) == 1 {
			serveJSON(w, http.StatusOK, []byte(`{"metadata":{"resourceVersion":"1"},"items":null}`))
			return
		}
		serveJSON(w, http.StatusOK, fixture(t, "applications.json"))
	}
	fs := newFakeServer(t, emptyOnTheSilentBaselinePoll)
	h := &activeArgo{
		client:   fs.client(fs.config()),
		cfg:      fs.config(),
		interval: 50 * time.Millisecond,
		state:    stream.StateOf(nil),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	events, err := h.Stream(ctx)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	select {
	case ev := <-events:
		if ev.Section.Err != nil {
			t.Fatalf("emission carried an error: %v", ev.Section.Err)
		}
		if len(ev.Section.Items) != 5 {
			t.Fatalf("emission had %d items, want all 5", len(ev.Section.Items))
		}
		if ev.Section.Items[0].Title != "billing-cron" {
			t.Errorf("first item = %q, want worst-first ordering preserved on the stream path",
				ev.Section.Items[0].Title)
		}
		if ev.Section.Items[0].Meta["state"] != stateFailed {
			t.Errorf("stream items lost their meta: %v", ev.Section.Items[0].Meta)
		}
	case <-ctx.Done():
		t.Fatal("no emission before the deadline")
	}
}

func TestStreamSurfacesErrorsAsSections(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		serveJSON(w, http.StatusForbidden, fixture(t, "error_403.json"))
	})
	h := &activeArgo{
		client:   fs.client(fs.config()),
		cfg:      fs.config(),
		interval: 50 * time.Millisecond,
		state:    stream.StateOf(nil),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	events, err := h.Stream(ctx)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	select {
	case ev := <-events:
		if ev.Section.Err == nil {
			t.Fatal("a 403 produced no section error; a broken stream must be visible, not silent")
		}
		if !strings.Contains(ev.Section.Err.Error(), "forbidden") {
			t.Errorf("section error = %v", ev.Section.Err)
		}
	case <-ctx.Done():
		t.Fatal("no emission before the deadline")
	}
}

func TestLatencyFloorReportsTheInterval(t *testing.T) {
	h := &activeArgo{interval: 90 * time.Second}
	if got := h.LatencyFloor(); got != 90*time.Second {
		t.Errorf("LatencyFloor = %s, want the configured interval", got)
	}
	if got := h.Name(); got != SignalName {
		t.Errorf("Name = %q, want %q", got, SignalName)
	}
}

func TestBackoffGrowsWithConsecutiveFailures(t *testing.T) {
	base := time.Minute
	one := httpx.Backoff(base, 1)
	two := httpx.Backoff(base, 2)
	four := httpx.Backoff(base, 4)
	if one != base {
		t.Errorf("Backoff(1) = %s, want the base interval", one)
	}
	if two <= one || four <= two {
		t.Errorf("Backoff did not grow: %s, %s, %s; a failing server would be hammered at full rate",
			one, two, four)
	}
	if got := httpx.Backoff(base, 99); got != httpx.MaxPollBackoff {
		t.Errorf("Backoff(99) = %s, want the %s cap", got, httpx.MaxPollBackoff)
	}
}

func TestRetryAfterWidensTheNextInterval(t *testing.T) {
	hdr := http.Header{}
	hdr.Set("Retry-After", "30")
	got := retryAfterFrom(hdr)
	if got != 30*time.Second {
		t.Fatalf("retryAfterFrom = %s, want 30s", got)
	}
	if retryAfterFrom(http.Header{}) != 0 {
		t.Error("a response with no Retry-After produced a delay")
	}
	if retryAfterFrom(nil) != 0 {
		t.Error("a nil header produced a delay")
	}
}
