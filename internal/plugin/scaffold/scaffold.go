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

const ExampleDirective = `name: scaffold-ping
type: query
signal: scaffold
params: {}
`
