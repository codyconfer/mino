package gcx

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/codyconfer/mino/plugin"
)

type actionRig struct {
	host     *fakeHost
	requests *atomic.Int32
	paths    []string
	bodies   []map[string]any
}

func newActionRig(t *testing.T, settings map[string]any) *actionRig {
	t.Helper()
	rig := &actionRig{host: newFakeHost(settings), requests: &atomic.Int32{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rig.requests.Add(1)
		rig.paths = append(rig.paths, r.URL.Path)
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		rig.bodies = append(rig.bodies, body)
		if strings.HasSuffix(r.URL.Path, "CreateIncident") {
			_, _ = w.Write(fixture(t, "incident_created.json"))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	rig.host.store[TokenKey] = plugin.Credential{AccessToken: "glsa_test"}
	clearEnv(t)
	pinStack(t, "example.grafana.net")

	prev := hostFn
	hostFn = func() plugin.Host { return rig.host }
	t.Cleanup(func() { hostFn = prev })

	prevClient := newClientFn
	newClientFn = func(stack, token string) (*Client, error) {
		host, err := normalizeStack(stack)
		if err != nil {
			return nil, err
		}
		return &Client{BaseURL: srv.URL, Stack: host, Token: token, HTTP: srv.Client()}, nil
	}
	t.Cleanup(func() { newClientFn = prevClient })

	return rig
}

func TestDeclareIncidentRequiresTitle(t *testing.T) {
	rig := newActionRig(t, map[string]any{"allow_write": true})
	err := declareIncident(context.Background(), map[string]string{"title": "  "})
	if err == nil {
		t.Fatal("expected an error")
	}
	if rig.requests.Load() != 0 {
		t.Fatal("a missing title must not open a connection")
	}
}

func TestAddActivityRequiresIncidentAndBody(t *testing.T) {
	rig := newActionRig(t, map[string]any{"allow_write": true})
	for _, p := range []map[string]string{
		{"body": "note"},
		{"incident": "incident-1"},
		{},
	} {
		if err := addActivity(context.Background(), p); err == nil {
			t.Fatalf("expected an error for %#v", p)
		}
	}
	if rig.requests.Load() != 0 {
		t.Fatal("missing params must not open a connection")
	}
}

func TestActionsRefuseWithoutAllowWrite(t *testing.T) {
	rig := newActionRig(t, nil)
	err := declareIncident(context.Background(), map[string]string{"title": "smoke"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(hintOf(err), "plugins.gcx.allow_write") {
		t.Fatalf("hint = %q", hintOf(err))
	}
	if rig.requests.Load() != 0 {
		t.Fatal("the write gate must refuse before any request")
	}
}

func TestDeclareIncidentHappyPath(t *testing.T) {
	rig := newActionRig(t, map[string]any{"allow_write": true, "default_severity": "major"})
	err := declareIncident(context.Background(), map[string]string{
		"title":   "API latency elevated",
		"summary": "p95 doubled",
		"labels":  "team:core, sev",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rig.requests.Load() != 1 {
		t.Fatalf("requests = %d", rig.requests.Load())
	}
	if rig.paths[0] != methodCreateIncident {
		t.Fatalf("path = %q", rig.paths[0])
	}
	body := rig.bodies[0]
	if body["title"] != "API latency elevated" || body["severity"] != "major" {
		t.Fatalf("body = %#v", body)
	}
	labels, _ := body["labels"].([]any)
	if len(labels) != 2 || labels[0] != "team:core" {
		t.Fatalf("labels = %#v", body["labels"])
	}
}

func TestAddActivityHappyPathAcceptsIDAlias(t *testing.T) {
	rig := newActionRig(t, map[string]any{"allow_write": true})
	if err := addActivity(context.Background(), map[string]string{"id": "incident-1", "body": "rolled back"}); err != nil {
		t.Fatal(err)
	}
	if rig.paths[0] != methodAddActivity {
		t.Fatalf("path = %q", rig.paths[0])
	}
	if rig.bodies[0]["incidentID"] != "incident-1" || rig.bodies[0]["activityKind"] != "userNote" {
		t.Fatalf("body = %#v", rig.bodies[0])
	}
}

func TestActionWithoutHost(t *testing.T) {
	clearEnv(t)
	prev := hostFn
	hostFn = func() plugin.Host { return nil }
	t.Cleanup(func() { hostFn = prev })

	err := declareIncident(context.Background(), map[string]string{"title": "smoke"})
	if err == nil || !strings.Contains(err.Error(), "no mino host") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunActionThroughRegistry(t *testing.T) {
	rig := newActionRig(t, map[string]any{"allow_write": true})
	err := plugin.RunAction(context.Background(), SignalName, "declare-incident",
		map[string]string{"title": "via registry"})
	if err != nil {
		t.Fatal(err)
	}
	if rig.requests.Load() != 1 {
		t.Fatalf("requests = %d", rig.requests.Load())
	}
	if rig.bodies[0]["title"] != "via registry" {
		t.Fatalf("body = %#v", rig.bodies[0])
	}
}
