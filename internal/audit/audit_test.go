package audit

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/codyconfer/munin/internal/signals"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "audit.duckdb"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRecordAndRecallFlight(t *testing.T) {
	s := openTemp(t)
	start := time.Now()

	flightID := s.StartFlight("triage", "oncall")
	if flightID == 0 {
		t.Fatal("StartFlight returned 0")
	}

	sections := []signals.Section{{
		Signal: "github",
		Title:  "Incidents",
		Items: []signals.Item{
			{Kind: "pr", Title: "sev2 outage", Subtitle: "org/repo", URL: "https://x/1", Timestamp: start},
			{Kind: "pr", Title: "sev3 blip", Subtitle: "org/repo", Timestamp: start},
		},
	}}
	s.RecordQuery(flightID, "incidents", "oncall", start, time.Now(), sections)
	s.FinishFlight(flightID)

	top, err := s.RecentEntries(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 1 || top[0].Kind != "flight" || top[0].Name != "triage" {
		t.Fatalf("RecentEntries = %+v", top)
	}
	if top[0].ItemCount != 2 {
		t.Errorf("flight item_count = %d, want 2 (rolled up from children)", top[0].ItemCount)
	}
	if top[0].Finished.IsZero() {
		t.Error("flight should have a finished_at after FinishFlight")
	}

	children, err := s.Children(flightID)
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 1 || children[0].Name != "incidents" || children[0].ItemCount != 2 {
		t.Fatalf("Children = %+v", children)
	}
	items, err := s.Items(children[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("Items = %d, want 2", len(items))
	}
}

func TestRecordQueryError(t *testing.T) {
	s := openTemp(t)
	now := time.Now()
	bad := []signals.Section{{Signal: "slack", Title: "slack", Err: errString("no token")}}
	s.RecordQuery(0, "slack-standup", "", now, now, bad)

	top, err := s.RecentEntries(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 1 || top[0].Error != "no token" {
		t.Fatalf("expected recorded error, got %+v", top)
	}
}

func TestNilStoreIsNoop(t *testing.T) {
	var s *Store

	if id := s.StartFlight("x", ""); id != 0 {
		t.Errorf("nil StartFlight = %d, want 0", id)
	}
	s.RecordQuery(0, "x", "", time.Now(), time.Now(), nil)
	s.FinishFlight(1)
	if runs, err := s.RecentEntries(5); err != nil || runs != nil {
		t.Errorf("nil RecentEntries = %v, %v", runs, err)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
