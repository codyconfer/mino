package kubectl

import (
	"context"
	"sync"
	"time"

	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/plugin"
)

const (
	PluginID    = "external.kubectl"
	SignalName  = "kubectl"
	GlyphID     = "external.kubectl"
	ContextTool = "kubectl"
)

const probeTTL = 30 * time.Second

var shared = &provider{}

func Register() {
	if _, ok := plugin.Lookup(PluginID); ok {
		return
	}
	plugin.RegisterSignal(plugin.Descriptor{
		ID:                 PluginID,
		Kind:               plugin.KindSignal,
		Signal:             SignalName,
		Capabilities:       []plugin.Capability{plugin.CapQuery, plugin.CapStream, plugin.CapCacheable},
		SettingsNamespaces: []string{SignalName},
	}, plugin.Builders{
		Query:  BuildQuery,
		Stream: BuildStream,
	})
	glyph.Register(GlyphID, glyph.Variants{Nerd: "󱃾", Uni: "⎈", ASCII: "k8"})
	plugin.RegisterContext(PluginID, shared)
	plugin.RegisterStatusContribution(PluginID, func(_, _ string) glyph.StatusContribution {
		return StatusContribution()
	})
	registerParams()
	registerCommand()
	plugin.RegisterSeeds(PluginID, []plugin.FileSeed{
		{RelPath: "queries/kubectl-context.yaml", Content: []byte(ExampleDirective)},
		{RelPath: "queries/kubectl-health.yaml", Content: []byte(HealthDirective)},
	})
}

type provider struct {
	mu   sync.Mutex
	last string

	probed   string
	probedOK bool
	probedAt time.Time

	unhealthy int
	graded    bool
}

func (p *provider) Tool() string { return ContextTool }

func (p *provider) Switch(_ context.Context, name string) error {
	p.mu.Lock()
	p.last = name
	p.mu.Unlock()
	return nil
}

func (p *provider) Current(ctx context.Context) (string, bool, error) {
	if name := p.selected(); name != "" {
		return name, true, nil
	}
	name, ok := p.currentFromKubeconfig(ctx)
	return name, ok, nil
}

func (p *provider) selected() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.last
}

func (p *provider) currentFromKubeconfig(ctx context.Context) (string, bool) {
	p.mu.Lock()
	if !p.probedAt.IsZero() && time.Since(p.probedAt) < probeTTL {
		name, ok := p.probed, p.probedOK
		p.mu.Unlock()
		return name, ok
	}
	p.mu.Unlock()

	name, ok := probeContext(ctx, DefaultBinary)

	p.mu.Lock()
	p.probed, p.probedOK, p.probedAt = name, ok, time.Now()
	p.mu.Unlock()
	return name, ok
}

func (p *provider) grade(unhealthy int) {
	p.mu.Lock()
	p.unhealthy, p.graded = unhealthy, true
	p.mu.Unlock()
}

func (p *provider) verdict() (unhealthy int, graded bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.unhealthy, p.graded
}

func StatusContribution() glyph.StatusContribution {
	return glyph.StatusContribution{
		BrandGlyph: glyph.ResolveID(GlyphID),
		Info:       func() string { return ContextTool },
		Status: func() (string, glyph.Severity) {
			name, ok, _ := shared.Current(context.Background())
			if !ok || name == "" {
				return glyph.StatusMuted(), glyph.SeverityNeutral
			}
			if unhealthy, graded := shared.verdict(); graded && unhealthy > 0 {
				return glyph.StatusWarn(), glyph.SeverityNegative
			}
			return glyph.StatusOK(), glyph.SeverityPositive
		},
	}
}

const ExampleDirective = `name: kubectl-context
type: query
signal: kubectl
params:
  what: context
`

const HealthDirective = `name: kubectl-health
type: query
signal: kubectl
params:
  what: pods,events,nodes,workloads
  since: 1h
  limit: 25
`
