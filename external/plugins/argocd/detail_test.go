package argocd

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/plugin"
)

func detailServer(t *testing.T, treeStatus int) *fakeServer {
	t.Helper()
	return newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/resource-tree"):
			if treeStatus != http.StatusOK {
				serveJSON(w, treeStatus, []byte(`{"error":"tree unavailable"}`))
				return
			}
			serveJSON(w, http.StatusOK, fixture(t, "resource_tree.json"))
		case strings.Contains(r.URL.Path, "/revisions/"):
			serveJSON(w, http.StatusOK, fixture(t, "revision_metadata.json"))
		default:
			serveJSON(w, http.StatusOK, fixture(t, "application.json"))
		}
	})
}

func sectionByTitle(d plugin.ItemDetail, title string) (plugin.DetailSection, bool) {
	for _, s := range d.Sections {
		if s.Title == title {
			return s, true
		}
	}
	return plugin.DetailSection{}, false
}

func TestDetailFromAFullyPopulatedItem(t *testing.T) {
	fs := detailServer(t, http.StatusOK)
	item := plugin.Item{
		Kind: itemKindApp,
		Meta: map[string]string{"app": "search-indexer", "app_namespace": "team-search"},
	}

	d, err := fs.signal(fs.config()).Detail(context.Background(), item)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if d.Title != "search-indexer" {
		t.Errorf("Title = %q", d.Title)
	}
	if d.Meta["state"] != stateInProgress {
		t.Errorf("meta[state] = %q, want %q for a Running operation", d.Meta["state"], stateInProgress)
	}
	if d.Meta["in_progress"] != "true" {
		t.Error("meta[in_progress] is not \"true\"; render.DetailHasInProgress reads exactly that key, so " +
			"a sync in flight would neither animate nor re-poll")
	}
}

func TestDetailFromAURLOnlyItem(t *testing.T) {
	fs := detailServer(t, http.StatusOK)
	item := plugin.Item{URL: fs.srv.URL + "/applications/team-search/search-indexer"}

	d, err := fs.signal(fs.config()).Detail(context.Background(), item)
	if err != nil {
		t.Fatalf("Detail from a URL-only item: %v; `mino show <url>` builds an Item with no Meta, so this "+
			"is the CLI path", err)
	}
	if d.Title != "search-indexer" {
		t.Errorf("Title = %q, want the app parsed out of the URL", d.Title)
	}
	if got := fs.at(0).query.Get("appNamespace"); got != "team-search" {
		t.Errorf("appNamespace = %q, want it recovered from the two-segment URL", got)
	}
}

func TestRefFromItemParsesBothURLForms(t *testing.T) {
	cases := []struct {
		raw       string
		name, ns  string
		wantError bool
	}{
		{raw: "https://argocd.example.com/applications/payments-api", name: "payments-api"},
		{raw: "https://argocd.example.com/applications/team-search/search-indexer", name: "search-indexer", ns: "team-search"},
		{raw: "https://argocd.example.com/settings/repos", wantError: true},
		{raw: "not a url at all", wantError: true},
	}
	s := &Signal{cfg: testConfig()}
	for _, c := range cases {
		t.Run(c.raw, func(t *testing.T) {
			name, ns, err := s.refFromItem(plugin.Item{URL: c.raw})
			if c.wantError {
				if err == nil {
					t.Fatalf("refFromItem(%q) = %q/%q, want an error naming the expected URL shape", c.raw, ns, name)
				}
				return
			}
			if err != nil {
				t.Fatalf("refFromItem(%q): %v", c.raw, err)
			}
			if name != c.name || ns != c.ns {
				t.Errorf("refFromItem = %q/%q, want %q/%q", ns, name, c.ns, c.name)
			}
		})
	}
}

func TestRefFromItemFallsBackToTheTitle(t *testing.T) {
	s := &Signal{cfg: Config{AppNamespace: "team-search"}}
	name, ns, err := s.refFromItem(plugin.Item{Title: "search-indexer"})
	if err != nil {
		t.Fatalf("refFromItem: %v", err)
	}
	if name != "search-indexer" || ns != "team-search" {
		t.Errorf("refFromItem = %q/%q, want the title plus the configured app_namespace", ns, name)
	}
}

func TestDetailChipsLeadWithHealth(t *testing.T) {
	fs := detailServer(t, http.StatusOK)
	d, err := fs.signal(fs.config()).Detail(context.Background(),
		plugin.Item{Meta: map[string]string{"app": "search-indexer", "app_namespace": "team-search"}})
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if len(d.Chips) != 3 {
		t.Fatalf("got %d chips, want health, sync and phase", len(d.Chips))
	}
	if d.Chips[0].Label != "Progressing" {
		t.Errorf("first chip = %q, want health; render.detailSeverity uses Chips[0] for the panel icon",
			d.Chips[0].Label)
	}
	if d.Chips[0].Sev != glyph.SeverityWarning {
		t.Errorf("Progressing severity = %v, want warning", d.Chips[0].Sev)
	}
	if d.Chips[1].Label != "OutOfSync" || d.Chips[1].Sev != glyph.SeverityWarning {
		t.Errorf("second chip = %+v, want OutOfSync/warning", d.Chips[1])
	}
}

func TestDetailResourcesSectionOptsIntoStateRows(t *testing.T) {
	fs := detailServer(t, http.StatusOK)
	d, err := fs.signal(fs.config()).Detail(context.Background(),
		plugin.Item{Meta: map[string]string{"app": "search-indexer", "app_namespace": "team-search"}})
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	sec, ok := sectionByTitle(d, "resources")
	if !ok {
		t.Fatal("no resources section")
	}
	if sec.Meta["state_rows"] != "true" {
		t.Error("resources section does not set meta[state_rows]; without it the rows render as plain " +
			"label/value pairs with no severity cue")
	}
	if len(sec.Rows) != 3 {
		t.Fatalf("got %d rows, want one per resource", len(sec.Rows))
	}
	if sec.Rows[0][0] != "Deployment/indexer" || sec.Rows[0][1] != "Progressing" {
		t.Errorf("first row = %v, want the resource-tree health to win over the sync status", sec.Rows[0])
	}
	if sec.Rows[2][1] != "Degraded" {
		t.Errorf("Job row = %v, want the tree's Degraded health rather than the OutOfSync sync status",
			sec.Rows[2])
	}
	if sec.Meta["in_progress"] != "true" {
		t.Error("resources section omits meta[in_progress] despite a Progressing resource")
	}
}

func TestResourceTreeFailureDegradesToANote(t *testing.T) {
	fs := detailServer(t, http.StatusInternalServerError)
	d, err := fs.signal(fs.config()).Detail(context.Background(),
		plugin.Item{Meta: map[string]string{"app": "search-indexer", "app_namespace": "team-search"}})
	if err != nil {
		t.Fatalf("Detail failed because the resource tree did: %v; the tree is supplementary and must not "+
			"take the whole panel down", err)
	}
	sec, ok := sectionByTitle(d, "resources")
	if !ok {
		t.Fatal("no resources section")
	}
	if len(sec.Rows) != 3 {
		t.Errorf("got %d rows, want the Application's own status.resources[] to still render", len(sec.Rows))
	}
	joined := strings.Join(sec.Lines, " ")
	if !strings.Contains(joined, "resource tree unavailable") {
		t.Errorf("lines = %v, want a note explaining the missing tree", sec.Lines)
	}
}

func TestDetailSections(t *testing.T) {
	fs := detailServer(t, http.StatusOK)
	d, err := fs.signal(fs.config()).Detail(context.Background(),
		plugin.Item{Meta: map[string]string{"app": "search-indexer", "app_namespace": "team-search"}})
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}

	if sec, ok := sectionByTitle(d, "sync history"); !ok {
		t.Error("no sync history section")
	} else if len(sec.Rows) != 2 || sec.Rows[0][0] != "2222222" {
		t.Errorf("history rows = %v, want newest first and shortened", sec.Rows)
	}

	sec, ok := sectionByTitle(d, "last operation")
	if !ok {
		t.Fatal("no last operation section")
	}
	if sec.Body != "syncing 3 resources" {
		t.Errorf("operation body = %q", sec.Body)
	}
	if !strings.Contains(strings.Join(sec.Lines, " "), "BackoffLimitExceeded") {
		t.Errorf("operation lines = %v, want the failed resource surfaced; that message is why the sync "+
			"is stuck", sec.Lines)
	}

	if sec, ok := sectionByTitle(d, "commit"); !ok {
		t.Error("no commit section despite resolvable revision metadata")
	} else if sec.Body != "search: bump indexer concurrency" {
		t.Errorf("commit body = %q", sec.Body)
	}

	if sec, ok := sectionByTitle(d, "conditions"); !ok {
		t.Error("no conditions section")
	} else if sec.Rows[0][0] != "SyncError" {
		t.Errorf("condition rows = %v", sec.Rows)
	}
}

func TestRevisionMetadataSkippedForAChartSource(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/revisions/") {
			t.Error("revision metadata requested for a Helm chart source; that endpoint is git-only and " +
				"the chart path is /chartdetails")
		}
		if strings.HasSuffix(r.URL.Path, "/resource-tree") {
			serveJSON(w, http.StatusOK, []byte(`{"nodes":[]}`))
			return
		}
		serveJSON(w, http.StatusOK, []byte(`{
			"metadata":{"name":"billing-cron","namespace":"argocd"},
			"spec":{"project":"platform","source":{"repoURL":"https://charts.acme.io","chart":"billing","targetRevision":"2.4.1"}},
			"status":{"sync":{"status":"Synced","revision":"2.4.1"},"health":{"status":"Healthy"}}
		}`))
	})

	d, err := fs.signal(fs.config()).Detail(context.Background(),
		plugin.Item{Meta: map[string]string{"app": "billing-cron"}})
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if _, ok := sectionByTitle(d, "commit"); ok {
		t.Error("commit section rendered for a chart source")
	}
}

func TestSettledApplicationHasNoInProgressFlag(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/resource-tree") {
			serveJSON(w, http.StatusOK, []byte(`{"nodes":[]}`))
			return
		}
		if strings.Contains(r.URL.Path, "/revisions/") {
			serveJSON(w, http.StatusOK, fixture(t, "revision_metadata.json"))
			return
		}
		serveJSON(w, http.StatusOK, []byte(`{
			"metadata":{"name":"payments-api","namespace":"argocd"},
			"spec":{"project":"platform","source":{"repoURL":"https://github.com/acme/deploy","path":"apps/payments-api"}},
			"status":{"sync":{"status":"Synced","revision":"9f1c2b7ad4e5f60718293a4b5c6d7e8f90123456"},
			          "health":{"status":"Healthy"},
			          "operationState":{"phase":"Succeeded","finishedAt":"2026-08-04T10:02:00Z"}}
		}`))
	})

	d, err := fs.signal(fs.config()).Detail(context.Background(),
		plugin.Item{Meta: map[string]string{"app": "payments-api"}})
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if _, bad := d.Meta["in_progress"]; bad {
		t.Error("a Succeeded application carries meta[in_progress]; the detail view would re-poll forever")
	}
	if d.Meta["state"] != stateSynced {
		t.Errorf("meta[state] = %q, want %q", d.Meta["state"], stateSynced)
	}
}

func TestDetailFailsWhenTheApplicationCallFails(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		serveJSON(w, http.StatusNotFound, []byte(`{"error":"applications.argoproj.io \"nope\" not found"}`))
	})
	_, err := fs.signal(fs.config()).Detail(context.Background(),
		plugin.Item{Meta: map[string]string{"app": "nope"}})
	if err == nil {
		t.Fatal("Detail succeeded for a missing application")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q", err)
	}
}
