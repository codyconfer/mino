// Package gooseai is a Lane C2 stub. Distinct from the author's goose game repo.
// Context write target: stub in-memory (future: goose session/model file).
package gooseai

import (
	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/munin/internal/plugin/external/stub"
)

const (
	PluginID   = "external.gooseai"
	SignalName = "gooseai"
)

var prov *stub.Provider

func init() {
	prov = stub.Register(stub.Spec{
		PluginID:   PluginID,
		SignalName: SignalName,
		Tool:       "gooseai",
		Title:      "gooseai",
		Glyph:      glyph.Variants{Nerd: "󰚩", Uni: "◇", ASCII: "ga"},
	})
}

func Signal() stub.Signal {
	return stub.Signal{NameStr: SignalName, Title: "gooseai", Prov: prov}
}

const ExampleDirective = `name: gooseai-context
signal: gooseai
params: {}
`
