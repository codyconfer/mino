package gcx

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &Client{
		BaseURL: srv.URL,
		Stack:   "example.grafana.net",
		Token:   "glsa_test",
		HTTP:    srv.Client(),
	}
}

func TestQueryIncidentsHappyPath(t *testing.T) {
	var gotPath, gotAuth, gotType string
	var gotBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth, gotType = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Content-Type")
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture(t, "incident_previews.json"))
	})

	incs, err := c.QueryIncidents(context.Background(), IncidentQuery{Status: StatusActive, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}

	if gotPath != methodQueryIncidentPreviews {
		t.Fatalf("path = %q want %q", gotPath, methodQueryIncidentPreviews)
	}
	if gotAuth != "Bearer glsa_test" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if gotType != "application/json" {
		t.Fatalf("content-type = %q", gotType)
	}
	query, _ := gotBody["query"].(map[string]any)
	if query["limit"] != float64(5) {
		t.Fatalf("limit = %v", query["limit"])
	}
	qs, _ := query["queryString"].(string)
	if !strings.Contains(qs, "status:active") || !strings.Contains(qs, "isdrill:false") {
		t.Fatalf("queryString = %q", qs)
	}

	if len(incs) != 2 {
		t.Fatalf("incidents = %d", len(incs))
	}
	if incs[0].ID != "incident-200" || incs[0].Severity != "critical" {
		t.Fatalf("incident0 = %#v", incs[0])
	}
	if !incs[1].Drill {
		t.Fatal("incident1 should be a drill")
	}
}

func TestQueryIncidentsSynthesizesURL(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(fixture(t, "incident_previews.json"))
	})
	incs, err := c.QueryIncidents(context.Background(), IncidentQuery{})
	if err != nil {
		t.Fatal(err)
	}
	want := "https://example.grafana.net/a/grafana-irm-app/incidents/incident-200"
	if incs[0].URL != want {
		t.Fatalf("URL = %q want %q", incs[0].URL, want)
	}
}

func TestQueryIncidentsClampsAndOmitsFilters(t *testing.T) {
	var query map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		query, _ = body["query"].(map[string]any)
		_, _ = w.Write([]byte(`{"incidentPreviews":[]}`))
	})
	if _, err := c.QueryIncidents(context.Background(), IncidentQuery{Status: StatusAll, Limit: 9999, IncludeDrills: true}); err != nil {
		t.Fatal(err)
	}
	if query["limit"] != float64(maxLimit) {
		t.Fatalf("limit = %v want %d", query["limit"], maxLimit)
	}
	if qs, _ := query["queryString"].(string); qs != "" {
		t.Fatalf("status=all with drills should send no terms, got %q", qs)
	}
}

func TestRPCUnauthorizedHint(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"invalid token"}`))
	})
	_, err := c.QueryIncidents(context.Background(), IncidentQuery{})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "invalid token") {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(hintOf(err), "mino login gcx") {
		t.Fatalf("hint = %q", hintOf(err))
	}
}

func TestRPCNotFoundHintNamesTheUnverifiedPath(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("no such route"))
	})
	_, err := c.QueryIncidents(context.Background(), IncidentQuery{})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(hintOf(err), "unverified") {
		t.Fatalf("hint = %q", hintOf(err))
	}
}

func TestRPCOversizeResponse(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, maxRPCResponseBytes+1))
	})
	if _, err := c.QueryIncidents(context.Background(), IncidentQuery{}); err == nil {
		t.Fatal("expected a bounded-read error")
	}
}

func TestRPCTimeout(t *testing.T) {
	release := make(chan struct{})
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	t.Cleanup(func() { close(release) })
	c.Timeout = 20 * time.Millisecond
	if _, err := c.QueryIncidents(context.Background(), IncidentQuery{}); err == nil {
		t.Fatal("expected a timeout error")
	}
}

func TestRPCMalformedResponse(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("{"))
	})
	if _, err := c.QueryIncidents(context.Background(), IncidentQuery{}); err == nil {
		t.Fatal("expected a decode error")
	}
}

func TestCreateIncident(t *testing.T) {
	var gotPath string
	var body map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = w.Write(fixture(t, "incident_created.json"))
	})
	inc, err := c.CreateIncident(context.Background(), NewIncident{
		Title: "API latency elevated", Severity: "major", Status: StatusActive,
		Summary: "p95 doubled", Labels: []string{"team:core"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != methodCreateIncident {
		t.Fatalf("path = %q want %q", gotPath, methodCreateIncident)
	}
	if body["title"] != "API latency elevated" || body["severity"] != "major" || body["summary"] != "p95 doubled" {
		t.Fatalf("body = %#v", body)
	}
	if body["isDrill"] != false {
		t.Fatalf("isDrill = %v", body["isDrill"])
	}
	if inc.ID != "incident-300" || inc.Severity != "major" {
		t.Fatalf("incident = %#v", inc)
	}
}

func TestAddActivity(t *testing.T) {
	var gotPath string
	var body map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = w.Write([]byte(`{}`))
	})
	if err := c.AddActivity(context.Background(), "incident-300", "rolled back", "userNote"); err != nil {
		t.Fatal(err)
	}
	if gotPath != methodAddActivity {
		t.Fatalf("path = %q want %q", gotPath, methodAddActivity)
	}
	if body["incidentID"] != "incident-300" || body["body"] != "rolled back" || body["activityKind"] != "userNote" {
		t.Fatalf("body = %#v", body)
	}
}

func TestNewClientValidates(t *testing.T) {
	if _, err := NewClient("", "glsa_x"); err == nil {
		t.Fatal("expected an error for an empty stack")
	}
	if _, err := NewClient("myorg.grafana.net", "  "); err == nil {
		t.Fatal("expected an error for an empty token")
	}
	c, err := NewClient("https://myorg.grafana.net/", "glsa_x")
	if err != nil {
		t.Fatal(err)
	}
	if c.Stack != "myorg.grafana.net" || c.BaseURL != "https://myorg.grafana.net"+IRMAPIPath {
		t.Fatalf("client = %#v", c)
	}
}
