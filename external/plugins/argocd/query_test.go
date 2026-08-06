package argocd

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestFetchMapsTheLiveResponse(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		serveJSON(w, http.StatusOK, fixture(t, "applications.json"))
	})

	secs, err := fs.signal(fs.config()).Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(secs) != 1 || len(secs[0].Items) != 5 {
		t.Fatalf("got %d sections with %d items, want 1 section of 5", len(secs), len(secs[0].Items))
	}
	if secs[0].Items[0].Title != "billing-cron" {
		t.Errorf("first item = %q, want the failed app first", secs[0].Items[0].Title)
	}
}

func TestFetchGroupsByProject(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		serveJSON(w, http.StatusOK, fixture(t, "applications.json"))
	})
	cfg := fs.config()
	cfg.GroupBy = groupByProject

	secs, err := fs.signal(cfg).Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(secs) != 2 {
		t.Fatalf("got %d sections, want one per project", len(secs))
	}
}

func TestFetchPropagatesTheHintOnFailure(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		serveJSON(w, http.StatusForbidden, fixture(t, "error_403.json"))
	})

	_, err := fs.signal(fs.config()).Fetch(context.Background())
	if err == nil {
		t.Fatal("Fetch swallowed a 403")
	}
	if !strings.Contains(err.Error(), "role:mino") {
		t.Errorf("error = %q, want the RBAC hint to survive the trip through Fetch; plugin errors carry "+
			"their hint inline, so losing it here leaves the user with no fix", err)
	}
}

func TestFetchOnAnEmptyInstance(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		serveJSON(w, http.StatusOK, []byte(`{"metadata":{"resourceVersion":"1"},"items":null}`))
	})

	secs, err := fs.signal(fs.config()).Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(secs) != 1 || len(secs[0].Items) != 0 {
		t.Fatalf("got %#v, want one empty section", secs)
	}
}

func TestSignalNameMatchesTheDescriptor(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, _ *http.Request) {})
	if got := fs.signal(fs.config()).Name(); got != SignalName {
		t.Errorf("Name = %q, want %q", got, SignalName)
	}
}
