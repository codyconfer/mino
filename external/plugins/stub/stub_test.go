package stub_test

import (
	"context"
	"testing"

	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/external/plugins/stub"
	"github.com/codyconfer/mino/plugin"
)

func TestRegisterWiresQueryAndContext(t *testing.T) {
	const id = "external.stubtest"
	const signal = "stubtest"
	const tool = "stubtest"
	if _, ok := plugin.Lookup(id); ok {
		t.Skip("stubtest already registered")
	}
	p := stub.Register(stub.Spec{
		PluginID:   id,
		SignalName: signal,
		Tool:       tool,
		Title:      "stubtest",
		Glyph:      glyph.Variants{Nerd: "S", Uni: "S", ASCII: "st"},
	})
	if !plugin.HasCapability(signal, plugin.CapQuery) {
		t.Fatal("expected CapQuery")
	}
	if err := plugin.SwitchContext(context.Background(), tool, "alpha"); err != nil {
		t.Fatal(err)
	}
	secs, err := stub.Signal{NameStr: signal, Title: "stubtest", Prov: p}.Fetch(context.Background())
	if err != nil || len(secs) != 1 || secs[0].Items[0].Body != "alpha" {
		t.Fatalf("Fetch = %#v err=%v", secs, err)
	}
}

func TestStatusContributionMutedAndOK(t *testing.T) {
	const id = "external.stubstatus"
	const signal = "stubstatus"
	const tool = "stubstatus"
	if _, ok := plugin.Lookup(id); ok {
		t.Skip("stubstatus already registered")
	}
	p := stub.Register(stub.Spec{
		PluginID:   id,
		SignalName: signal,
		Tool:       tool,
		Title:      "stubstatus",
		Glyph:      glyph.Variants{Nerd: "T", Uni: "T", ASCII: "ss"},
	})
	contrib := stub.StatusContribution(id, tool, p)
	if contrib.Info == nil || contrib.Info() != tool {
		t.Fatalf("Info want %q", tool)
	}
	muted, sev := contrib.Status()
	if muted != glyph.StatusMuted() || sev != glyph.SeverityNeutral {
		t.Fatalf("unset status = %q sev=%v", muted, sev)
	}
	if err := p.Switch(context.Background(), "beta"); err != nil {
		t.Fatal(err)
	}
	okGlyph, okSev := contrib.Status()
	if okGlyph != glyph.StatusOK() || okSev != glyph.SeverityPositive {
		t.Fatalf("set status = %q sev=%v", okGlyph, okSev)
	}
	nilContrib := stub.StatusContribution(id, tool, nil)
	muted, sev = nilContrib.Status()
	if muted != glyph.StatusMuted() || sev != glyph.SeverityNeutral {
		t.Fatalf("nil provider status = %q sev=%v", muted, sev)
	}
}
