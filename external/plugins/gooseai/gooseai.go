// Package gooseai is a Lane C2 stub. Distinct from the author's goose game repo.
// Context write target: stub in-memory (future: goose session/model file).
package gooseai

import (
	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/external/plugins/stub"
	"github.com/codyconfer/mino/plugin"
)

const (
	PluginID    = "external.gooseai"
	SignalName  = "gooseai"
	GlyphID     = "external.gooseai"
	ContextTool = "gooseai"
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
		Title:      "gooseai",
		Glyph:      glyph.Variants{Nerd: "󰚩", Uni: "◇", ASCII: "ga"},
	})
	plugin.RegisterSeeds(PluginID, []plugin.FileSeed{
		{RelPath: "queries/gooseai-context.yaml", Content: []byte(ExampleDirective)},
	})
}

func Signal() stub.Signal {
	return stub.Signal{NameStr: SignalName, Title: "gooseai", Prov: prov}
}

// StatusContribution exposes the current context chip for the status strip.
func StatusContribution() glyph.StatusContribution {
	return stub.StatusContribution(GlyphID, ContextTool, prov)
}

const ExampleDirective = `name: gooseai-context
signal: gooseai
params: {}
`
