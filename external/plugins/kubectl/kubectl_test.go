package kubectl

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

func TestStubRegistration(t *testing.T) {
	d, ok := plugin.Lookup(PluginID)
	if !ok || d.Signal != SignalName {
		t.Fatalf("lookup = %+v ok=%v", d, ok)
	}
	if _, ok := glyph.Named(GlyphID); !ok {
		t.Fatal("glyph missing")
	}
	secs, err := (Signal{}).Fetch(context.Background())
	if err != nil || len(secs) != 1 {
		t.Fatalf("Fetch = %v err=%v", secs, err)
	}
	contrib := StatusContribution()
	if contrib.Info == nil || contrib.Info() != ContextTool {
		t.Fatalf("Info want %q", ContextTool)
	}
}

func TestSharedProviderUsedByFetch(t *testing.T) {
	prev := shared.last
	shared.last = ""
	defer func() { shared.last = prev }()

	want := "mino-shared-provider-ctx"
	if err := plugin.SwitchContext(context.Background(), ContextTool, want); err != nil {
		t.Fatal(err)
	}
	if shared.last != want {
		t.Fatalf("SwitchContext did not update shared provider instance: last=%q want=%q", shared.last, want)
	}

	secs, err := (Signal{}).Fetch(context.Background())
	if err != nil || len(secs) != 1 || len(secs[0].Items) == 0 {
		t.Fatalf("Fetch = %v err=%v", secs, err)
	}
	if body := secs[0].Items[0].Body; body != want {
		t.Fatalf("Fetch body = %q, want in-process %q", body, want)
	}
}

func TestSwitchDoesNotRequireKubectlBinary(t *testing.T) {
	prev := shared.last
	shared.last = ""
	defer func() { shared.last = prev }()

	if err := shared.Switch(context.Background(), "definitely-not-a-real-context-xyz"); err != nil {
		t.Fatalf("Switch = %v", err)
	}
	if shared.last != "definitely-not-a-real-context-xyz" {
		t.Fatalf("last = %q", shared.last)
	}
}

func TestFixtureContextDrivesFetchAndStatus(t *testing.T) {
	fx := loadContextFixture(t)
	prev := shared.last
	defer func() { shared.last = prev }()

	shared.last = fx.Context
	secs, err := (Signal{}).Fetch(context.Background())
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

	contrib := StatusContribution()
	okGlyph, okSev := contrib.Status()
	if okGlyph != glyph.StatusOK() || okSev != glyph.SeverityPositive {
		t.Fatalf("set status = %q sev=%v", okGlyph, okSev)
	}

	shared.last = ""
	muted, sev := contrib.Status()
	if muted == glyph.StatusOK() && sev == glyph.SeverityPositive {
		t.Skip("live kubectl current-context present; muted path not exercisable offline")
	}
	if muted != glyph.StatusMuted() || sev != glyph.SeverityNeutral {
		t.Fatalf("unset status = %q sev=%v", muted, sev)
	}
}

func TestExampleDirective(t *testing.T) {
	want := "name: kubectl-context\nsignal: kubectl\nparams: {}\n"
	if ExampleDirective != want {
		t.Fatalf("ExampleDirective = %q", ExampleDirective)
	}
}

func TestMain(m *testing.M) {
	Register()
	os.Exit(m.Run())
}
