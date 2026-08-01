// Package opencode is a Lane C2 stub. Context write target: stub in-memory (future: opencode project/cwd).
package opencode

import (
	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/external/plugins/stub"
	"github.com/codyconfer/mino/plugin"
)

const (
	PluginID    = "external.opencode"
	SignalName  = "opencode"
	GlyphID     = "external.opencode"
	ContextTool = "opencode"
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
		Title:      "opencode",
		Glyph:      glyph.Variants{Nerd: "󰨞", Uni: "⌘", ASCII: "oc"},
	})
	plugin.RegisterSeeds(PluginID, []plugin.FileSeed{
		{RelPath: "queries/opencode-context.yaml", Content: []byte(ExampleDirective)},
	})
}

func Signal() stub.Signal {
	return stub.Signal{NameStr: SignalName, Title: "opencode", Prov: prov}
}

// StatusContribution exposes the current context chip for the status strip.
func StatusContribution() glyph.StatusContribution {
	return stub.StatusContribution(GlyphID, ContextTool, prov)
}

const ExampleDirective = `name: opencode-context
signal: opencode
params: {}
`
