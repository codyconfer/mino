// Package pi is a Lane C2 stub. Context write target: stub in-memory (future: pi model env).
package pi

import (
	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/external/plugins/stub"
	"github.com/codyconfer/mino/plugin"
)

const (
	PluginID    = "external.pi"
	SignalName  = "pi"
	GlyphID     = "external.pi"
	ContextTool = "pi"
)

var prov *stub.Provider

func Register() {
	if _, ok := plugin.Lookup(PluginID); ok {
		return
	}
	prov = stub.Register(stub.Spec{
		PluginID:   PluginID,
		SignalName: SignalName,
		Tool:       ContextTool,
		Title:      "pi",
		Glyph:      glyph.Variants{Nerd: "", Uni: "π", ASCII: "pi"},
	})
	plugin.RegisterSeeds(PluginID, []plugin.FileSeed{
		{RelPath: "queries/pi-context.yaml", Content: []byte(ExampleDirective)},
	})
}

func Signal() stub.Signal {
	return stub.Signal{NameStr: SignalName, Title: "pi", Prov: prov}
}

// StatusContribution exposes the current context chip for the status strip.
func StatusContribution() glyph.StatusContribution {
	return stub.StatusContribution(GlyphID, ContextTool, prov)
}

const ExampleDirective = `name: pi-context
signal: pi
params: {}
`
