package gcx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/plugin"
)

type memTokens map[string]struct{ token, scope string }

func (m memTokens) Get(_ context.Context, service string) (string, string, bool, error) {
	c, ok := m[service]
	return c.token, c.scope, ok, nil
}

func TestRegistry(t *testing.T) {
	d, ok := plugin.Lookup(PluginID)
	if !ok || d.Signal != SignalName {
		t.Fatalf("lookup = %+v ok=%v", d, ok)
	}
	if _, ok := glyph.Named(GlyphID); !ok {
		t.Fatal("glyph missing")
	}
	var hasQuery, hasAction bool
	for _, c := range d.Capabilities {
		if c == plugin.CapQuery {
			hasQuery = true
		}
		if c == plugin.CapAction {
			hasAction = true
		}
	}
	if !hasQuery || !hasAction {
		t.Fatalf("capabilities = %v", d.Capabilities)
	}
	if len(plugin.ActionsFor(SignalName)) == 0 {
		t.Fatal("expected RegisterAction bindings for CapAction")
	}
	_ = StatusContribution()
}

func TestFetchAuthAndContext(t *testing.T) {
	shared.cur = ""
	defer func() { shared.cur = "" }()

	secs, err := NewSignal(nil).Fetch(context.Background())
	if err != nil || len(secs) != 1 || len(secs[0].Items) != 3 {
		t.Fatalf("Fetch = %#v err=%v", secs, err)
	}
	if secs[0].Items[0].Body == "" || secs[0].Items[1].Body != "(unset)" {
		t.Fatalf("unexpected items: %#v", secs[0].Items)
	}

	tok := memTokens{TokenKey: {token: "glsa_test", scope: "irm"}}
	if err := plugin.SwitchContext(context.Background(), ContextTool, "example.grafana.net"); err != nil {
		t.Fatal(err)
	}
	secs, err = NewSignal(tok).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if secs[0].Items[0].Body != "sealed key gcx: present scope=irm" {
		t.Fatalf("auth body = %q", secs[0].Items[0].Body)
	}
	if secs[0].Items[1].Body != "example.grafana.net" {
		t.Fatalf("stack = %q", secs[0].Items[1].Body)
	}
}

func TestSharedProviderUsedByFetch(t *testing.T) {
	shared.cur = ""
	defer func() { shared.cur = "" }()
	if err := plugin.SwitchContext(context.Background(), ContextTool, "myorg.grafana.net"); err != nil {
		t.Fatal(err)
	}
	secs, err := NewSignal(nil).Fetch(context.Background())
	if err != nil || len(secs) == 0 {
		t.Fatalf("Fetch = %v err=%v", secs, err)
	}
	found := false
	for _, it := range secs[0].Items {
		if it.Kind == "context" && strings.Contains(it.Body, "myorg.grafana.net") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Fetch did not use registered context: %#v", secs[0].Items)
	}
}

func TestMapIncidentsJSONFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "incidents_list.json"))
	if err != nil {
		t.Fatal(err)
	}
	sec, err := MapIncidentsJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if sec.Signal != SignalName || sec.Title != "incidents" || len(sec.Items) != 2 {
		t.Fatalf("section = %#v", sec)
	}
	if sec.Items[0].Title != "API latency elevated" || sec.Items[0].Meta["id"] != "incident-100" {
		t.Fatalf("item0 = %#v", sec.Items[0])
	}
	if sec.Items[1].URL == "" {
		t.Fatal("expected incident URL")
	}
}

func TestStubActions(t *testing.T) {
	acts := KnownActions()
	if len(acts) != 2 {
		t.Fatalf("KnownActions = %d", len(acts))
	}
	if acts[0].Name() != "declare-incident" || acts[1].Name() != "add-activity" {
		t.Fatalf("action names = %q %q", acts[0].Name(), acts[1].Name())
	}
	if err := acts[0].Run(context.Background(), nil); err == nil {
		t.Fatal("expected stub error")
	}
	if err := acts[1].Run(context.Background(), nil); err == nil {
		t.Fatal("expected stub error for add-activity")
	}
	if err := plugin.RunAction(context.Background(), SignalName, "declare-incident", nil); err == nil {
		t.Fatal("expected registered stub to error")
	}
	if err := plugin.RunAction(context.Background(), SignalName, "add-activity", nil); err == nil {
		t.Fatal("expected registered stub to error")
	}
}

func TestMain(m *testing.M) {
	Register()
	os.Exit(m.Run())
}
