// Package opencode is a Lane C2 stub. Context write target: stub in-memory (future: opencode project/cwd).
package opencode

import (
	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/munin/internal/plugin/external/stub"
)

const (
	PluginID   = "external.opencode"
	SignalName = "opencode"
)

var prov *stub.Provider

func init() {
	prov = stub.Register(stub.Spec{
		PluginID: PluginID, SignalName: SignalName, Tool: "opencode", Title: "opencode",
		Glyph: glyph.Variants{Nerd: "󰨞", Uni: "⌘", ASCII: "oc"},
	})
}

func Signal() stub.Signal {
	return stub.Signal{NameStr: SignalName, Title: "opencode", Prov: prov}
}

const ExampleDirective = `name: opencode-context
signal: opencode
params: {}
`
