package verify

import (
	"context"
	"testing"

	"github.com/codyconfer/mino/internal/plugin"
	_ "github.com/codyconfer/mino/internal/plugin/ntr"
	_ "github.com/codyconfer/mino/internal/render"
	"github.com/codyconfer/mino/internal/signals"
	"github.com/codyconfer/mino/internal/signals/build"
)

type contextProbe struct{}

func (contextProbe) Tool() string { return "verify-context-probe" }
func (contextProbe) Switch(context.Context, string) error {
	return nil
}
func (contextProbe) Current(context.Context) (string, bool, error) {
	return "", false, nil
}

type probeQuery struct{}

func (probeQuery) Name() string { return "contextprobe" }
func (probeQuery) Fetch(context.Context) ([]signals.Section, error) {
	return nil, nil
}

func TestPluginsCoversAllKinds(t *testing.T) {
	plugin.RegisterBuiltins()
	_ = build.BuilderSignals()

	const probeID = "test.verify.contextprobe"
	if _, ok := plugin.Lookup(probeID); !ok {
		plugin.RegisterSignal(plugin.Descriptor{
			ID:           probeID,
			Kind:         plugin.KindSignal,
			Signal:       "contextprobe",
			Capabilities: []plugin.Capability{plugin.CapQuery},
		}, plugin.Builders{
			Query: func(plugin.BuildContext) (plugin.Query, error) {
				return probeQuery{}, nil
			},
		})
		plugin.RegisterContext(probeID, contextProbe{})
		plugin.RegisterFilterEngine(probeID, "contextprobe-filter", func(items []signals.Item) []signals.Item { return items })
	}

	seen := map[plugin.Kind]bool{}
	findings := Plugins()
	var problems []Finding
	for _, f := range findings {
		if !f.OK {
			problems = append(problems, f)
		}
	}
	if len(problems) > 0 {
		for _, f := range problems {
			t.Errorf("%s: %s", f.Name, f.Msg)
		}
	}

	for _, d := range plugin.All() {
		seen[d.Kind] = true
	}
	for _, k := range plugin.KnownKinds() {
		if !seen[k] {
			t.Errorf("no registered descriptor for Kind %q (routing gap)", k)
		}
	}
}
