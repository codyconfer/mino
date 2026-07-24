// Package ollama is a Lane C2 stub. Context write target: stub in-memory (future: OLLAMA_MODEL / API).
package ollama

import (
	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/munin/internal/plugin/external/stub"
)

const (
	PluginID   = "external.ollama"
	SignalName = "ollama"
)

var prov *stub.Provider

func init() {
	prov = stub.Register(stub.Spec{
		PluginID: PluginID, SignalName: SignalName, Tool: "ollama", Title: "ollama",
		Glyph: glyph.Variants{Nerd: "󰳆", Uni: "◎", ASCII: "ol"},
	})
}

func Signal() stub.Signal {
	return stub.Signal{NameStr: SignalName, Title: "ollama", Prov: prov}
}

const ExampleDirective = `name: ollama-context
signal: ollama
params: {}
`
