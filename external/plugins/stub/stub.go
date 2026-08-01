// Package stub helpers for Lane C2 template instantiations.
package stub

import (
	"context"

	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/plugin"
)

// Spec describes a minimal external plugin stub.
type Spec struct {
	PluginID   string
	SignalName string
	Tool       string
	Glyph      glyph.Variants
	Title      string
}

// Register installs registry, builders, glyph, context provider, and status contribution.
func Register(s Spec) *Provider {
	p := &Provider{tool: s.Tool}
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           s.PluginID,
		Kind:         plugin.KindSignal,
		Signal:       s.SignalName,
		Capabilities: []plugin.Capability{plugin.CapQuery},
	}, plugin.Builders{
		Query: func(plugin.BuildContext) (plugin.Query, error) {
			return Signal{NameStr: s.SignalName, Title: s.Title, Prov: p}, nil
		},
	})
	if !glyph.Register(s.PluginID, s.Glyph) {
		plugin.NoteDiagnostic(s.PluginID, plugin.KindSignal, s.PluginID,
			"glyph id already registered; keeping the incumbent glyph")
	}
	plugin.RegisterContext(s.PluginID, p)
	plugin.RegisterStatusContribution(s.PluginID, func(_, _ string) glyph.StatusContribution {
		return StatusContribution(s.PluginID, s.Tool, p)
	})
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

// StatusContribution exposes context-set health for the status strip.
func StatusContribution(glyphID, tool string, p *Provider) glyph.StatusContribution {
	return glyph.StatusContribution{
		BrandGlyph: glyph.ResolveID(glyphID),
		Info:       func() string { return tool },
		Status: func() (string, glyph.Severity) {
			if p == nil {
				return glyph.StatusMuted(), glyph.SeverityNeutral
			}
			name, ok, _ := p.Current(context.Background())
			if !ok || name == "" {
				return glyph.StatusMuted(), glyph.SeverityNeutral
			}
			return glyph.StatusOK(), glyph.SeverityPositive
		},
	}
}

// Signal returns one canned item showing the active context.
type Signal struct {
	NameStr string
	Title   string
	Prov    *Provider
}

func (s Signal) Name() string { return s.NameStr }

func (s Signal) Fetch(ctx context.Context) ([]plugin.Section, error) {
	cur := "(unset)"
	if s.Prov != nil {
		if n, ok, _ := s.Prov.Current(ctx); ok && n != "" {
			cur = n
		}
	}
	return []plugin.Section{{
		Signal: s.NameStr,
		Title:  s.Title,
		Items: []plugin.Item{{
			Kind:  "context",
			Title: s.Title + " context",
			Body:  cur,
		}},
	}}, nil
}
