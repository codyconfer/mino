package argocd

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/plugin"
)

func clearContext(t *testing.T) {
	t.Helper()
	if err := shared.Switch(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	shared.observe("")
	shared.noteAuth(false)
	t.Cleanup(func() {
		_ = shared.Switch(context.Background(), "")
		shared.observe("")
		shared.noteAuth(false)
	})
}

func TestRegistry(t *testing.T) {
	d, ok := plugin.Lookup(PluginID)
	if !ok || d.Signal != SignalName {
		t.Fatalf("lookup = %+v ok=%v", d, ok)
	}
	for _, cap := range []plugin.Capability{plugin.CapQuery, plugin.CapStream, plugin.CapDetail, plugin.CapCacheable} {
		if !plugin.HasCapability(SignalName, cap) {
			t.Errorf("missing capability %q", cap)
		}
	}
	if !contains(d.Credentials, TokenKey) {
		t.Errorf("Credentials = %v, want it to contain %q; a non-empty list replaces the signal-name "+
			"default, so omitting it locks the plugin out of its own token", d.Credentials, TokenKey)
	}
	if !contains(d.SettingsNamespaces, SignalName) {
		t.Errorf("SettingsNamespaces = %v, want it to contain %q, or every settings read returns nil",
			d.SettingsNamespaces, SignalName)
	}
	if _, ok := glyph.Named(GlyphID); !ok {
		t.Error("glyph missing")
	}
	if !plugin.HasStreamBuilder(SignalName) {
		t.Error("no stream builder registered despite CapStream")
	}
	if len(plugin.QueryParams(SignalName)) == 0 {
		t.Error("no query params registered; the CLI and completion would offer nothing")
	}
}

func TestQueryBuilderResultAlsoImplementsDetailer(t *testing.T) {
	q, err := BuildQuery(buildCtx{settings: map[string]any{"server_url": testServer}})
	if err != nil {
		t.Fatalf("BuildQuery: %v", err)
	}
	if _, ok := q.(plugin.Detailer); !ok {
		t.Fatalf("BuildQuery returned %T, which does not implement plugin.Detailer; build.Detail obtains "+
			"the detailer by type-asserting the query builder's result, so detail would fail at runtime", q)
	}
}

func TestQueryBuilderNeedsNoParams(t *testing.T) {
	if _, err := BuildQuery(buildCtx{settings: map[string]any{"server_url": testServer}}); err != nil {
		t.Fatalf("BuildQuery with nil params: %v; build.Detail calls the query builder with params == nil, "+
			"so a required param would break detail for every item", err)
	}
}

func TestQueryBuilderRefusesAMissingServerURL(t *testing.T) {
	_, err := BuildQuery(buildCtx{})
	if err == nil {
		t.Fatal("BuildQuery accepted an unconfigured plugin; it would fail later with a confusing HTTP error")
	}
	if !strings.Contains(err.Error(), "server_url") {
		t.Errorf("error = %q, want it to name server_url so the user knows what to set", err)
	}
}

func TestStatusContributionReflectsConfig(t *testing.T) {
	clearContext(t)

	contrib := StatusContribution()
	if contrib.Info == nil {
		t.Fatal("status contribution has no Info function")
	}
	if got := contrib.Info(); got != ContextTool {
		t.Fatalf("Info() = %q, want %q", got, ContextTool)
	}
	if got, sev := contrib.Status(); got != glyph.StatusMuted() || sev != glyph.SeverityNeutral {
		t.Errorf("unconfigured status = %q sev=%v, want muted/neutral", got, sev)
	}

	shared.observe(testServer)
	if got, sev := contrib.Status(); got != glyph.StatusMuted() || sev != glyph.SeverityNeutral {
		t.Errorf("server-without-token status = %q sev=%v, want muted; a server URL alone cannot read "+
			"anything", got, sev)
	}

	shared.noteAuth(true)
	if got, sev := contrib.Status(); got != glyph.StatusOK() || sev != glyph.SeverityPositive {
		t.Errorf("configured status = %q sev=%v, want ok/positive", got, sev)
	}
}

func TestContextProviderFallsBackToTheConfiguredServer(t *testing.T) {
	clearContext(t)
	shared.observe(testServer)

	name, ok, err := shared.Current(context.Background())
	if err != nil || !ok || name != "argocd.example.com" {
		t.Fatalf("Current = %q ok=%v err=%v, want the configured host so the chip means something "+
			"with zero extra config", name, ok, err)
	}

	if err := plugin.SwitchContext(context.Background(), ContextTool, "staging.argocd.example.com"); err != nil {
		t.Fatalf("SwitchContext: %v", err)
	}
	name, _, _ = shared.Current(context.Background())
	if name != "staging.argocd.example.com" {
		t.Errorf("Current = %q, want the explicit selection to win over the configured server", name)
	}
}

func TestExampleDirectivesDeclareAType(t *testing.T) {
	for name, directive := range map[string]string{
		"ExampleDirective":          ExampleDirective,
		"ExampleUnhealthyDirective": ExampleUnhealthyDirective,
	} {
		if !strings.Contains(directive, "type: query") {
			t.Errorf("%s omits `type:`; the directive loader rejects it outright, so `mino plugins install "+
				"external.argocd` would break every directive in the home", name)
		}
		if !strings.Contains(directive, "signal: argocd") {
			t.Errorf("%s does not name the argocd signal", name)
		}
	}
}

func TestMain(m *testing.M) {
	Register()
	os.Exit(m.Run())
}
