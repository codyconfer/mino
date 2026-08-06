package argocd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/plugin"
)

const testServer = "https://argocd.example.com"

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func testConfig() Config {
	return Config{ServerURL: testServer, TokenEnv: DefaultTokenEnv, Max: defaultMax, GroupBy: groupByNone}
}

func mapFixture(t *testing.T, cfg Config) []plugin.Section {
	t.Helper()
	secs, err := MapApplicationsJSON(fixture(t, "applications.json"), cfg)
	if err != nil {
		t.Fatalf("MapApplicationsJSON: %v", err)
	}
	return secs
}

func itemsByTitle(secs []plugin.Section) map[string]plugin.Item {
	out := map[string]plugin.Item{}
	for _, s := range secs {
		for _, it := range s.Items {
			out[it.Title] = it
		}
	}
	return out
}

func TestMapApplicationsProducesOneSection(t *testing.T) {
	secs := mapFixture(t, testConfig())
	if len(secs) != 1 {
		t.Fatalf("got %d sections, want 1 when group_by is none", len(secs))
	}
	if secs[0].Signal != SignalName {
		t.Errorf("section signal = %q, want %q", secs[0].Signal, SignalName)
	}
	if len(secs[0].Items) != 5 {
		t.Fatalf("got %d items, want all 5 fixture applications", len(secs[0].Items))
	}
	if got := secs[0].Meta["server"]; got != "argocd.example.com" {
		t.Errorf("section meta server = %q, want the bare host so the panel header is readable", got)
	}
	if got := secs[0].Meta["total"]; got != "5" {
		t.Errorf("section meta total = %q, want 5", got)
	}
}

func TestArgoStateCoversEveryBranch(t *testing.T) {
	cases := []struct {
		name string
		app  application
		want string
	}{
		{"operation failed beats a healthy rollup", application{
			Status: applicationStatus{
				Sync:           syncStatus{Status: "Synced"},
				Health:         healthStatus{Status: "Healthy"},
				OperationState: &operationState{Phase: "Failed"},
			}}, stateFailed},
		{"operation error", application{
			Status: applicationStatus{OperationState: &operationState{Phase: "Error"}}}, stateFailed},
		{"degraded", application{
			Status: applicationStatus{Sync: syncStatus{Status: "Synced"}, Health: healthStatus{Status: "Degraded"}}}, stateDegraded},
		{"missing", application{
			Status: applicationStatus{Health: healthStatus{Status: "Missing"}}}, stateMissing},
		{"running operation", application{
			Status: applicationStatus{Health: healthStatus{Status: "Healthy"}, OperationState: &operationState{Phase: "Running"}}}, stateInProgress},
		{"terminating operation", application{
			Status: applicationStatus{OperationState: &operationState{Phase: "Terminating"}}}, stateInProgress},
		{"progressing health", application{
			Status: applicationStatus{Health: healthStatus{Status: "Progressing"}}}, stateProgressing},
		{"out of sync", application{
			Status: applicationStatus{Sync: syncStatus{Status: "OutOfSync"}, Health: healthStatus{Status: "Healthy"}}}, stateOutOfSync},
		{"suspended", application{
			Status: applicationStatus{Sync: syncStatus{Status: "Synced"}, Health: healthStatus{Status: "Suspended"}}}, stateSuspended},
		{"synced and healthy", application{
			Status: applicationStatus{Sync: syncStatus{Status: "Synced"}, Health: healthStatus{Status: "Healthy"}}}, stateSynced},
		{"nothing known", application{}, stateUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := argoState(c.app); got != c.want {
				t.Errorf("argoState = %q, want %q; the rollup drives severity, ordering and the spinner, "+
					"so a wrong branch mislabels the whole row", got, c.want)
			}
		})
	}
}

func TestItemMetaCarriesTheLoadBearingKeys(t *testing.T) {
	items := itemsByTitle(mapFixture(t, testConfig()))

	payments, ok := items["payments-api"]
	if !ok {
		t.Fatal("payments-api missing from the mapped items")
	}
	want := map[string]string{
		"state":           stateSynced,
		"app":             "payments-api",
		"app_namespace":   "argocd",
		"project":         "platform",
		"cluster":         "in-cluster",
		"namespace":       "payments",
		"sync":            "Synced",
		"health":          "Healthy",
		"phase":           "Succeeded",
		"revision":        "9f1c2b7ad4e5f60718293a4b5c6d7e8f90123456",
		"revision_short":  "9f1c2b7",
		"repo":            "https://github.com/acme/deploy",
		"path":            "apps/payments-api",
		"target_revision": "main",
		"initiated_by":    "release-bot",
		"sync_started":    "2026-08-04T10:00:00Z",
		"sync_finished":   "2026-08-04T10:02:00Z",
	}
	for k, v := range want {
		if got := payments.Meta[k]; got != v {
			t.Errorf("meta[%q] = %q, want %q", k, got, v)
		}
	}
	if payments.Kind != itemKindApp {
		t.Errorf("Kind = %q, want %q", payments.Kind, itemKindApp)
	}
	if payments.Subtitle != "platform · in-cluster/payments" {
		t.Errorf("Subtitle = %q; project must come first because render.ItemScope takes the segment "+
			"before the separator", payments.Subtitle)
	}
	if !payments.Timestamp.Equal(time.Date(2026, 8, 4, 10, 2, 30, 0, time.UTC)) {
		t.Errorf("Timestamp = %s, want the newest of the operation and reconcile stamps", payments.Timestamp)
	}
}

func TestItemMetaNeverSetsGithubOnlyKeys(t *testing.T) {
	for title, it := range itemsByTitle(mapFixture(t, testConfig())) {
		if _, bad := it.Meta["conclusion"]; bad {
			t.Errorf("%s sets meta[conclusion]; on a workflow-shaped item that key suppresses the spinner "+
				"and it means nothing for ArgoCD", title)
		}
		if _, bad := it.Meta["status"]; bad {
			t.Errorf("%s sets meta[status]; render.detailMetaRows lists both state and status, so setting "+
				"both renders a duplicate row", title)
		}
	}
}

func TestInProgressFlagTracksTheRollup(t *testing.T) {
	items := itemsByTitle(mapFixture(t, testConfig()))
	if got := items["search-indexer"].Meta["in_progress"]; got != "true" {
		t.Errorf("search-indexer meta[in_progress] = %q, want \"true\"; a running sync must animate", got)
	}
	for _, settled := range []string{"payments-api", "checkout-web", "billing-cron", "legacy-shim"} {
		if _, bad := items[settled].Meta["in_progress"]; bad {
			t.Errorf("%s carries meta[in_progress] while settled; the deck would poll it forever", settled)
		}
	}
}

func TestWorstFirstOrdering(t *testing.T) {
	secs := mapFixture(t, testConfig())
	want := []string{"billing-cron", "checkout-web", "search-indexer", "legacy-shim", "payments-api"}
	got := make([]string, 0, len(secs[0].Items))
	for _, it := range secs[0].Items {
		got = append(got, it.Title)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ordering = %v, want %v; the broken applications belong at the top of the deck", got, want)
		}
	}
}

func TestEmptyItemListIsNotAPanic(t *testing.T) {
	secs, err := MapApplicationsJSON([]byte(`{"metadata":{},"items":null}`), testConfig())
	if err != nil {
		t.Fatalf("MapApplicationsJSON on an empty instance: %v", err)
	}
	if len(secs) != 1 || len(secs[0].Items) != 0 {
		t.Fatalf("got %#v, want one empty section; ArgoCD sends items:null, not []", secs)
	}
	if secs[0].Meta["total"] != "0" {
		t.Errorf("total = %q, want 0", secs[0].Meta["total"])
	}
}

func TestMultiSourceAppFallsBackToTheFirstSource(t *testing.T) {
	it := itemsByTitle(mapFixture(t, testConfig()))["search-indexer"]
	if it.Meta["repo"] != "https://github.com/acme/deploy" {
		t.Errorf("repo = %q; an app using spec.sources[] must still report its source", it.Meta["repo"])
	}
	if it.Meta["path"] != "apps/search" {
		t.Errorf("path = %q, want apps/search", it.Meta["path"])
	}
}

func TestChartRevisionIsNotShortened(t *testing.T) {
	it := itemsByTitle(mapFixture(t, testConfig()))["billing-cron"]
	if it.Meta["revision_short"] != "2.4.1" {
		t.Errorf("revision_short = %q, want the chart version verbatim; truncating it to 7 characters "+
			"would corrupt a semver", it.Meta["revision_short"])
	}
}

func TestDeepLinkUsesTheTwoSegmentFormOutsideTheDefaultNamespace(t *testing.T) {
	items := itemsByTitle(mapFixture(t, testConfig()))
	if got := items["payments-api"].URL; got != testServer+"/applications/payments-api" {
		t.Errorf("URL = %q, want the one-segment form for an app in the argocd namespace", got)
	}
	if got := items["search-indexer"].URL; got != testServer+"/applications/team-search/search-indexer" {
		t.Errorf("URL = %q; the one-segment form 404s for apps-in-any-namespace", got)
	}
}

func TestNamespacesFilterIsAppliedClientSide(t *testing.T) {
	cfg := testConfig()
	cfg.Namespaces = []string{"payments", "search"}
	secs := mapFixture(t, cfg)
	if len(secs[0].Items) != 2 {
		t.Fatalf("got %d items, want 2; namespaces filters on spec.destination.namespace, which the "+
			"list API cannot filter server-side", len(secs[0].Items))
	}
	for _, it := range secs[0].Items {
		if it.Meta["namespace"] != "payments" && it.Meta["namespace"] != "search" {
			t.Errorf("kept %s in namespace %q", it.Title, it.Meta["namespace"])
		}
	}
}

func TestOnlyUnhealthyDropsSyncedAndHealthyApps(t *testing.T) {
	cfg := testConfig()
	cfg.OnlyUnhealthy = true
	secs := mapFixture(t, cfg)
	for _, it := range secs[0].Items {
		if it.Meta["state"] == stateSynced {
			t.Errorf("only_unhealthy kept %s, which is synced and healthy", it.Title)
		}
	}
	if len(secs[0].Items) != 4 {
		t.Fatalf("got %d items, want 4", len(secs[0].Items))
	}
}

func TestMaxTruncatesAfterSortingAndReportsTheRemainder(t *testing.T) {
	cfg := testConfig()
	cfg.Max = 2
	secs := mapFixture(t, cfg)
	if len(secs[0].Items) != 2 {
		t.Fatalf("got %d items, want 2", len(secs[0].Items))
	}
	if secs[0].Items[0].Title != "billing-cron" {
		t.Errorf("first item = %q; max must apply after the worst-first sort or truncation hides the "+
			"failures it exists to surface", secs[0].Items[0].Title)
	}
	if secs[0].Meta[plugin.MetaMore] != "3" {
		t.Errorf("meta[more] = %q, want 3", secs[0].Meta[plugin.MetaMore])
	}
	if secs[0].Meta[plugin.MetaTruncated] != "true" {
		t.Errorf("meta[truncated] = %q, want true", secs[0].Meta[plugin.MetaTruncated])
	}
}

func TestGroupByProjectYieldsOneSectionPerProject(t *testing.T) {
	cfg := testConfig()
	cfg.GroupBy = groupByProject
	secs := mapFixture(t, cfg)
	if len(secs) != 2 {
		t.Fatalf("got %d sections, want one per project", len(secs))
	}
	seen := map[string]bool{}
	for _, s := range secs {
		seen[s.Title] = true
		for _, it := range s.Items {
			if SignalName+" · "+it.Meta["project"] != s.Title {
				t.Errorf("item %s (project %q) landed in section %q", it.Title, it.Meta["project"], s.Title)
			}
		}
	}
	for _, want := range []string{SignalName + " · platform", SignalName + " · storefront"} {
		if !seen[want] {
			t.Errorf("missing section %q, got %v", want, seen)
		}
	}
}

func TestSeverityForMatchesTheRollupVocabulary(t *testing.T) {
	cases := map[string]glyph.Severity{
		stateSynced:      glyph.SeverityPositive,
		"Healthy":        glyph.SeverityPositive,
		stateFailed:      glyph.SeverityNegative,
		stateDegraded:    glyph.SeverityNegative,
		stateMissing:     glyph.SeverityNegative,
		stateInProgress:  glyph.SeverityWarning,
		stateProgressing: glyph.SeverityWarning,
		stateOutOfSync:   glyph.SeverityWarning,
		"OutOfSync":      glyph.SeverityWarning,
		stateSuspended:   glyph.SeverityNeutral,
		stateUnknown:     glyph.SeverityNeutral,
		"":               glyph.SeverityNeutral,
	}
	for state, want := range cases {
		if got := severityFor(state); got != want {
			t.Errorf("severityFor(%q) = %v, want %v; detail chips colour themselves and must not depend "+
				"on the host classifier", state, got, want)
		}
	}
}
