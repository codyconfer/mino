// Package ollama is a Lane C2 stub. Context write target: stub in-memory (future: OLLAMA_MODEL / API).
package ollama

import (
	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/external/plugins/stub"
	"github.com/codyconfer/mino/plugin"
)

const (
	PluginID    = "external.ollama"
	SignalName  = "ollama"
	GlyphID     = "external.ollama"
	ContextTool = "ollama"
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
		Title:      "ollama",
		Glyph:      glyph.Variants{Nerd: "󰳆", Uni: "◎", ASCII: "ol"},
	})
	plugin.RegisterSeeds(PluginID, []plugin.FileSeed{
		{RelPath: "queries/ollama-context.yaml", Content: []byte(ExampleDirective)},
	})
}

func Signal() stub.Signal {
	return stub.Signal{NameStr: SignalName, Title: "ollama", Prov: prov}
}

// StatusContribution exposes the current context chip for the status strip.
func StatusContribution() glyph.StatusContribution {
	return stub.StatusContribution(GlyphID, ContextTool, prov)
}

const ExampleDirective = `name: ollama-context
signal: ollama
params: {}
`
