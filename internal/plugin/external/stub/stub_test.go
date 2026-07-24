package stub_test

import (
	"context"
	"testing"

	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/plugin/external/stub"
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
