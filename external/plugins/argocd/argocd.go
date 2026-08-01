// Package argocd is a Lane C2 stub. Context write target: stub in-memory (future: argocd context / config current-context).
package argocd

import (
	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/external/plugins/stub"
	"github.com/codyconfer/mino/plugin"
)

const (
	PluginID    = "external.argocd"
	SignalName  = "argocd"
	GlyphID     = "external.argocd"
	ContextTool = "argocd"
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
		Title:      "argocd",
		Glyph:      glyph.Variants{Nerd: "󱓞", Uni: "◈", ASCII: "ac"},
	})
	plugin.RegisterSeeds(PluginID, []plugin.FileSeed{
		{RelPath: "queries/argocd-context.yaml", Content: []byte(ExampleDirective)},
	})
}

func Signal() stub.Signal {
	return stub.Signal{NameStr: SignalName, Title: "argocd", Prov: prov}
}

// StatusContribution exposes the current context chip for the status strip.
func StatusContribution() glyph.StatusContribution {
	return stub.StatusContribution(GlyphID, ContextTool, prov)
}

const ExampleDirective = `name: argocd-context
signal: argocd
params: {}
`
