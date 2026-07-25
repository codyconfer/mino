// Package scaffold is the canonical plugin template.
//
// Use `munin plugins scaffold <id> --dir <path>` to generate an overlay-friendly
// package from [Generate] (public munin/plugin SDK). This package remains the
// in-tree reference implementation for hosts/tests.
//
// A complete plugin typically provides:
//   - plugin.RegisterSignal(Descriptor{…}, Builders{…}) for registry + builders + verify
//   - Query (signals.Signal) and optionally Stream / Action / Scheduled
//   - glyph.Register + plugin.RegisterStatusContribution for status strip
//   - plugin.RegisterContext when the tool has switchable context
//   - plugin.RegisterFilterEngine (or RegisterFilter) for KindFilter contributions
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
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           PluginID,
		Kind:         plugin.KindSignal,
		Signal:       SignalName,
		Capabilities: []plugin.Capability{plugin.CapQuery},
	}, plugin.Builders{
		Query: func(plugin.BuildContext) (plugin.Query, error) {
			return Signal{}, nil
		},
	})
	glyph.Register(GlyphID, glyph.Variants{Nerd: "", Uni: "◇", ASCII: "sc"})
	plugin.RegisterContext(PluginID, &provider{})
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
