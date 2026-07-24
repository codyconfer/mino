// Package scaffold is the canonical plugin template (ADR-14).
// Copy this package when adding a new signal plugin. Stubs are template
// instantiations — there is no generator CLI yet.
//
// A complete plugin typically provides:
//   - plugin.Register(Descriptor{…}) for compile-time registry + verify
//   - Query (signals.Signal) and optionally Stream / Action / Scheduled
//   - glyph.Register for brand/status contribution
//   - plugin.RegisterContextProvider when the tool has switchable context
//   - a fixture test + example directive YAML
package scaffold

import (
	"context"

	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/signals"
)

const (
	PluginID    = "scaffold.example"
	SignalName  = "scaffold"
	GlyphID     = "scaffold.example"
	ContextTool = "scaffold"
)

// Register installs the example plugin contributions. Hosts and tests call
// this explicitly; production binaries do not import scaffold by default.
func Register() {
	if _, ok := plugin.Lookup(PluginID); ok {
		return
	}
	plugin.Register(plugin.Descriptor{
		ID:           PluginID,
		Kind:         plugin.KindSignal,
		Signal:       SignalName,
		Capabilities: []plugin.Capability{plugin.CapQuery},
	})
	glyph.Register(GlyphID, glyph.Variants{Nerd: "", Uni: "◇", ASCII: "sc"})
	plugin.RegisterContextProvider(&provider{})
}

// Signal is a minimal Query capability implementation.
type Signal struct{}

func (Signal) Name() string { return SignalName }

func (Signal) Fetch(context.Context) ([]signals.Section, error) {
	return []signals.Section{{
		Signal: SignalName,
		Title:  SignalName,
		Items: []signals.Item{{
			Kind:  "scaffold",
			Title: "scaffold plugin is alive",
		}},
	}}, nil
}


type provider struct{ current string }

func (p *provider) Tool() string { return ContextTool }

func (p *provider) Switch(_ context.Context, name string) error {
	p.current = name
	return nil
}

func (p *provider) Current(context.Context) (string, bool, error) {
	if p.current == "" {
		return "", false, nil
	}
	return p.current, true, nil
}

// ExampleDirective is a sample query YAML body for docs/fixtures.
const ExampleDirective = `name: scaffold-ping
signal: scaffold
params: {}
`
