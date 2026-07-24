// Package stub helpers for Lane C2 template instantiations.
package stub

import (
	"context"

	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/signals"
)

// Spec describes a minimal external plugin stub.
type Spec struct {
	PluginID   string
	SignalName string
	Tool       string
	Glyph      glyph.Variants
	Title      string
}

// Register installs registry, glyph, and a last-switch context provider.
func Register(s Spec) *Provider {
	plugin.Register(plugin.Descriptor{
		ID:           s.PluginID,
		Kind:         plugin.KindSignal,
		Signal:       s.SignalName,
		Capabilities: []plugin.Capability{plugin.CapQuery},
	})
	glyph.Register(s.PluginID, s.Glyph)
	p := &Provider{tool: s.Tool}
	plugin.RegisterContextProvider(p)
	return p
}

// Provider records Switch/Current in-memory (write target documented per plugin).
type Provider struct {
	tool string
	cur  string
}

func (p *Provider) Tool() string { return p.tool }

func (p *Provider) Switch(_ context.Context, name string) error {
	p.cur = name
	return nil
}

func (p *Provider) Current(context.Context) (string, bool, error) {
	if p.cur == "" {
		return "", false, nil
	}
	return p.cur, true, nil
}

// Signal returns one canned item showing the active context.
type Signal struct {
	NameStr string
	Title   string
	Prov    *Provider
}

func (s Signal) Name() string { return s.NameStr }

func (s Signal) Fetch(ctx context.Context) ([]signals.Section, error) {
	cur := "(unset)"
	if s.Prov != nil {
		if n, ok, _ := s.Prov.Current(ctx); ok && n != "" {
			cur = n
		}
	}
	return []signals.Section{{
		Signal: s.NameStr,
		Title:  s.Title,
		Items: []signals.Item{{
			Kind:  "context",
			Title: s.Title + " context",
			Body:  cur,
		}},
	}}, nil
}
