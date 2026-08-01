// Package example is a sample overlay signal plugin.
// It imports only the public mino/plugin SDK — never mino/internal.
package example

import (
	"context"

	"github.com/codyconfer/mino/plugin"
)

const (
	PluginID   = "overlay.example"
	SignalName = "example"
)

type query struct{}

func (query) Name() string { return SignalName }

func (query) Fetch(context.Context) ([]plugin.Section, error) {
	return []plugin.Section{{
		Signal: SignalName,
		Title:  SignalName,
		Items: []plugin.Item{{
			Kind:  "info",
			Title: "overlay example plugin is alive",
		}},
	}}, nil
}

// Register installs overlay plugin contributions for verify + host builders.
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
			return query{}, nil
		},
	})
}
