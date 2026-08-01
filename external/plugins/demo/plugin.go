package demo

import (
	"regexp"
	"strings"

	"github.com/codyconfer/mino/plugin"
)

const (
	PluginID   = "external.demo"
	SignalName = "demo"
)

func Register() {
	if _, ok := plugin.Lookup(PluginID); ok {
		return
	}
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           PluginID,
		Kind:         plugin.KindSignal,
		Signal:       SignalName,
		Capabilities: []plugin.Capability{plugin.CapQuery, plugin.CapStream},
	}, plugin.Builders{
		Query: func(plugin.BuildContext) (plugin.Query, error) {
			return Signal{}, nil
		},
		Stream: func(plugin.BuildContext) (plugin.Stream, error) {
			return Signal{}, nil
		},
	})
	plugin.RegisterFilterEngine(PluginID, "demo-no-lorem", noLoremEngine)
}

var loremNoise = regexp.MustCompile(`(?i)\b(lorem|ipsum)\b`)

func noLoremEngine(items []plugin.Item) []plugin.Item {
	out := make([]plugin.Item, 0, len(items))
	for _, it := range items {
		if strings.TrimSpace(it.Title) == "" {
			continue
		}
		if loremNoise.MatchString(it.Body) || loremNoise.MatchString(it.Title) {
			continue
		}
		out = append(out, it)
	}
	return out
}
