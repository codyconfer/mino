package flight

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/audit"
)

func TestFetchGroupsFlushesRealPerQueryTimes(t *testing.T) {
	au, err := audit.Open(context.Background(), filepath.Join(t.TempDir(), "audit.duckdb"))
	if err != nil {
		t.Fatalf("audit.Open: %v", err)
	}
	t.Cleanup(func() { au.Close() })

	parentID := au.StartFlight("triage", "oncall")
	queries := []Query{
		{Label: "quick", Src: fakeSignal{name: "quick", items: []string{"a", "b"}}},
		{Label: "slow", Src: fakeSignal{name: "slow", items: []string{"c"}, delay: 60 * time.Millisecond}},
	}
	groups := FetchGroups(context.Background(), au, "oncall", time.Second, queries, parentID)
	au.FinishFlight(parentID)

	if len(groups) != 2 {
		t.Fatalf("FetchGroups returned %d groups, want 2", len(groups))
	}

	children, err := au.Children(parentID)
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	if len(children) != 2 {
		t.Fatalf("recorded %d child runs, want 2", len(children))
	}
	byName := map[string]audit.AuditRow{}
	for _, c := range children {
		byName[c.Name] = c
	}
	quick, slow := byName["quick"], byName["slow"]
	if quick.ItemCount != 2 || slow.ItemCount != 1 {
		t.Errorf("item counts = quick %d, slow %d; want 2 and 1", quick.ItemCount, slow.ItemCount)
	}
	if quick.Role != "oncall" || slow.Role != "oncall" {
		t.Errorf("roles = %q and %q, want oncall", quick.Role, slow.Role)
	}
	if !quick.Finished.Before(slow.Finished.Add(-30 * time.Millisecond)) {
		t.Errorf("per-query finish times collapsed to the flush time: quick %v, slow %v",
			quick.Finished, slow.Finished)
	}
	if items, err := au.Items(quick.ID); err != nil || len(items) != 2 {
		t.Errorf("Items(quick) = %d items, %v; want 2", len(items), err)
	}
}

func TestFetchQueryRecordsWithoutAFlight(t *testing.T) {
	au, err := audit.Open(context.Background(), filepath.Join(t.TempDir(), "audit.duckdb"))
	if err != nil {
		t.Fatalf("audit.Open: %v", err)
	}
	t.Cleanup(func() { au.Close() })

	FetchQuery(context.Background(), au, "", time.Second,
		Query{Label: "solo", Src: fakeSignal{name: "solo", items: []string{"a"}}}, 0)

	runs, err := au.RecentEntries(10)
	if err != nil {
		t.Fatalf("RecentEntries: %v", err)
	}
	if len(runs) != 1 || runs[0].Name != "solo" || runs[0].ItemCount != 1 {
		t.Fatalf("RecentEntries = %+v", runs)
	}
}

func TestFetchGroupsToleratesNilAuditStore(t *testing.T) {
	groups := FetchGroups(context.Background(), nil, "", time.Second,
		[]Query{{Label: "q", Src: fakeSignal{name: "q", items: []string{"a"}}}}, 0)
	if len(groups) != 1 || len(groups[0].Sections) != 1 {
		t.Fatalf("FetchGroups with a nil store = %+v", groups)
	}
}
