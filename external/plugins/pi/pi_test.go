package pi

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/plugin"
)

type contextFixture struct {
	Context string `json:"context"`
	Signal  string `json:"signal"`
	Title   string `json:"title"`
	Kind    string `json:"kind"`
}

func loadContextFixture(t *testing.T) contextFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "context.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fx contextFixture
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	return fx
}

func clearContext(t *testing.T) {
	t.Helper()
	if err := prov.Switch(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
}

func TestStubRegistration(t *testing.T) {
	d, ok := plugin.Lookup(PluginID)
	if !ok || d.Signal != SignalName {
		t.Fatalf("lookup = %+v ok=%v", d, ok)
	}
	if _, ok := glyph.Named(GlyphID); !ok {
		t.Fatal("glyph missing")
	}
	if !plugin.HasCapability(SignalName, plugin.CapQuery) {
		t.Fatal("expected CapQuery")
	}
	secs, err := Signal().Fetch(context.Background())
	if err != nil || len(secs) != 1 {
		t.Fatalf("Fetch = %v err=%v", secs, err)
	}
}

func TestFixtureContextDrivesFetchAndStatus(t *testing.T) {
	fx := loadContextFixture(t)
	clearContext(t)
	t.Cleanup(func() { _ = prov.Switch(context.Background(), "") })

	contrib := StatusContribution()
	if contrib.Info == nil || contrib.Info() != ContextTool {
		t.Fatalf("Info want %q", ContextTool)
	}
	muted, sev := contrib.Status()
	if muted != glyph.StatusMuted() || sev != glyph.SeverityNeutral {
		t.Fatalf("unset status = %q sev=%v", muted, sev)
	}

	if err := plugin.SwitchContext(context.Background(), ContextTool, fx.Context); err != nil {
		t.Fatal(err)
	}
	secs, err := Signal().Fetch(context.Background())
	if err != nil || len(secs) != 1 || len(secs[0].Items) != 1 {
		t.Fatalf("Fetch = %#v err=%v", secs, err)
	}
	item := secs[0].Items[0]
	if secs[0].Signal != fx.Signal || secs[0].Title != fx.Title {
		t.Fatalf("section = %#v", secs[0])
	}
	if item.Kind != fx.Kind || item.Body != fx.Context {
		t.Fatalf("item = %#v want kind=%q body=%q", item, fx.Kind, fx.Context)
	}
	okGlyph, okSev := contrib.Status()
	if okGlyph != glyph.StatusOK() || okSev != glyph.SeverityPositive {
		t.Fatalf("set status = %q sev=%v", okGlyph, okSev)
	}
}

func TestExampleDirective(t *testing.T) {
	want := "name: pi-context\nsignal: pi\nparams: {}\n"
	if ExampleDirective != want {
		t.Fatalf("ExampleDirective = %q", ExampleDirective)
	}
}

func TestMain(m *testing.M) {
	Register()
	os.Exit(m.Run())
}
