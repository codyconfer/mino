// Package pi is a Lane C2 stub. Context write target: stub in-memory (future: pi model env).
package pi

import (
	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/munin/internal/plugin/external/stub"
)

const (
	PluginID   = "external.pi"
	SignalName = "pi"
)

var prov *stub.Provider

func init() {
	prov = stub.Register(stub.Spec{
		PluginID:   PluginID,
		SignalName: SignalName,
		Tool:       "pi",
		Title:      "pi",
		Glyph:      glyph.Variants{Nerd: "", Uni: "π", ASCII: "pi"},
	})
}

func Signal() stub.Signal {
	return stub.Signal{NameStr: SignalName, Title: "pi", Prov: prov}
}

const ExampleDirective = `name: pi-context
signal: pi
params: {}
`
