// Package kubectl is a Lane C2 stub (template instantiation).
// Context writes are in-process only — role activation must not mutate the
// real kubeconfig. Optional Current() may probe `kubectl config current-context`
// read-only when no in-process selection exists.
package kubectl

import (
	"context"
	"os/exec"
	"strings"

	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/signals"
)

const (
	PluginID    = "external.kubectl"
	SignalName  = "kubectl"
	GlyphID     = "external.kubectl"
	ContextTool = "kubectl"
)

// shared is the single context provider instance used by Switch, Fetch, and status.
var shared = &provider{}

func init() {
	plugin.Register(plugin.Descriptor{
		ID:           PluginID,
		Kind:         plugin.KindSignal,
		Signal:       SignalName,
		Capabilities: []plugin.Capability{plugin.CapQuery},
	})
	glyph.Register(GlyphID, glyph.Variants{Nerd: "󱃾", Uni: "⎈", ASCII: "k8"})
	plugin.RegisterContextProvider(shared)
}

// Signal returns a canned cluster context snapshot.
type Signal struct{}

func (Signal) Name() string { return SignalName }

func (Signal) Fetch(ctx context.Context) ([]signals.Section, error) {
	cur, _, _ := shared.Current(ctx)
	if cur == "" {
		cur = "(no context)"
	}
	return []signals.Section{{
		Signal: SignalName,
		Title:  "kubectl",
		Items: []signals.Item{{
			Kind:  "context",
			Title: "current context",
			Body:  cur,
		}},
	}}, nil
}

type provider struct{ last string }

func (p *provider) Tool() string { return ContextTool }

// Switch records the selection in-process only (Lane C2 stub discipline).
// It never runs `kubectl config use-context` — that would mutate the user's
// kubeconfig on role load via ApplyRoleContexts.
func (p *provider) Switch(_ context.Context, name string) error {
	p.last = name
	return nil
}

func (p *provider) Current(ctx context.Context) (string, bool, error) {
	if p.last != "" {
		return p.last, true, nil
	}
	// Read-only probe when nothing has been switched in-process.
	if _, err := exec.LookPath("kubectl"); err != nil {
		return "", false, nil
	}
	out, err := exec.CommandContext(ctx, "kubectl", "config", "current-context").Output()
	if err != nil {
		return "", false, nil
	}
	cur := strings.TrimSpace(string(out))
	if cur == "" {
		return "", false, nil
	}
	return cur, true, nil
}

// StatusContribution exposes the current context chip.
func StatusContribution() glyph.StatusContribution {
	return glyph.StatusContribution{
		BrandGlyph: glyph.ResolveID(GlyphID),
		Info:       func() string { return ContextTool },
		Status: func() (string, glyph.Severity) {
			name, ok, _ := shared.Current(context.Background())
			if !ok || name == "" {
				return glyph.StatusMuted(), glyph.SeverityNeutral
			}
			return glyph.StatusOK(), glyph.SeverityPositive
		},
	}
}

// ExampleDirective for Lane D.
const ExampleDirective = `name: kubectl-context
signal: kubectl
params: {}
`
