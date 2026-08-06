package argocd

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/codyconfer/viewkit/glyph"
	"github.com/spf13/cobra"

	"github.com/codyconfer/mino/cmd"
	"github.com/codyconfer/mino/external/plugins/internal/params"
	"github.com/codyconfer/mino/external/plugins/internal/stream"
	"github.com/codyconfer/mino/plugin"
)

const (
	PluginID        = "external.argocd"
	SignalName      = "argocd"
	GlyphID         = "external.argocd"
	ContextTool     = "argocd"
	TokenKey        = "argocd"
	DefaultTokenEnv = "ARGOCD_AUTH_TOKEN"

	defaultInterval = 60 * time.Second
)

var shared = &provider{}

func Register() {
	if _, ok := plugin.Lookup(PluginID); ok {
		return
	}
	plugin.RegisterSignal(plugin.Descriptor{
		ID:     PluginID,
		Kind:   plugin.KindSignal,
		Signal: SignalName,
		Capabilities: []plugin.Capability{
			plugin.CapQuery, plugin.CapStream, plugin.CapDetail, plugin.CapCacheable,
		},
		Credentials:        []string{TokenKey},
		SettingsNamespaces: []string{SignalName},
	}, plugin.Builders{
		Query:  BuildQuery,
		Stream: BuildStream,
	})
	glyph.Register(GlyphID, glyph.Variants{Nerd: "󱓞", Uni: "◈", ASCII: "ac"})
	plugin.RegisterContext(PluginID, shared)
	plugin.RegisterStatusContribution(PluginID, func(_, _ string) glyph.StatusContribution {
		return StatusContribution()
	})
	plugin.RegisterQueryParams(SignalName,
		plugin.ParamSpec{Key: "app", Desc: "single application to read", Example: "payments-api"},
		plugin.ParamSpec{Key: "project", Desc: "ArgoCD project filter", Example: "platform", Delim: ","},
		plugin.ParamSpec{Key: "selector", Desc: "label selector on the Application", Example: "env=prod"},
		plugin.ParamSpec{Key: "app_namespace", Desc: "namespace holding the Application resource", Example: "argocd"},
		plugin.ParamSpec{Key: "namespace", Desc: "destination namespace filter", Example: "payments", Delim: ","},
		plugin.ParamSpec{Key: "only_unhealthy", Desc: "only apps that are not Synced and Healthy",
			Example: "true", Values: []string{"true", "false"}},
		plugin.ParamSpec{Key: "group_by", Desc: "one section per project or cluster",
			Example: "project", Values: []string{"none", "project", "cluster"}},
		plugin.ParamSpec{Key: "max", Desc: "maximum applications to return",
			Example: "50", Values: []string{"10", "25", "50", "100"}},
	)
	plugin.RegisterSeeds(PluginID, []plugin.FileSeed{
		{RelPath: "queries/argocd-apps.yaml", Content: []byte(ExampleDirective)},
		{RelPath: "queries/argocd-unhealthy.yaml", Content: []byte(ExampleUnhealthyDirective)},
	})
	cmd.RegisterCommand(newArgocdCmd)
}

func BuildQuery(bc plugin.BuildContext) (plugin.Query, error) {
	cfg, err := configFrom(bc)
	if err != nil {
		return nil, err
	}
	shared.observe(cfg.ServerURL)
	return New(cfg, tokenLookupFrom(bc)), nil
}

func BuildStream(bc plugin.BuildContext) (plugin.Stream, error) {
	interval, err := params.PollInterval(bc.Params(), SignalName, defaultInterval)
	if err != nil {
		return nil, err
	}
	cfg, err := configFrom(bc)
	if err != nil {
		return nil, err
	}
	shared.observe(cfg.ServerURL)
	return NewActive(cfg, tokenLookupFrom(bc), interval, stream.StateOf(bc)), nil
}

func newArgocdCmd() *cobra.Command {
	return cmd.SignalCmd(SignalName, "ArgoCD application health (read-only)")
}

type provider struct {
	mu       sync.RWMutex
	selected string
	server   string
	authed   bool
}

func (p *provider) Tool() string { return ContextTool }

func (p *provider) Switch(_ context.Context, name string) error {
	p.mu.Lock()
	p.selected = strings.TrimSpace(name)
	p.mu.Unlock()
	return nil
}

func (p *provider) Current(context.Context) (string, bool, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.selected != "" {
		return p.selected, true, nil
	}
	if p.server != "" {
		return p.server, true, nil
	}
	return "", false, nil
}

func (p *provider) observe(serverURL string) {
	host := serverHost(serverURL)
	p.mu.Lock()
	p.server = host
	p.mu.Unlock()
}

func (p *provider) noteAuth(ok bool) {
	p.mu.Lock()
	p.authed = ok
	p.mu.Unlock()
}

func (p *provider) authenticated() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.authed
}

func StatusContribution() glyph.StatusContribution {
	return glyph.StatusContribution{
		BrandGlyph: glyph.ResolveID(GlyphID),
		Info:       func() string { return ContextTool },
		Status: func() (string, glyph.Severity) {
			name, ok, _ := shared.Current(context.Background())
			if !ok || name == "" || !shared.authenticated() {
				return glyph.StatusMuted(), glyph.SeverityNeutral
			}
			return glyph.StatusOK(), glyph.SeverityPositive
		},
	}
}

const ExampleDirective = `name: argocd-apps
type: query
signal: argocd
params: {}
`

const ExampleUnhealthyDirective = `name: argocd-unhealthy
type: query
signal: argocd
params:
  only_unhealthy: "true"
`
