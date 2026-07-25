package plugin

import (
	"sort"
	"sync"

	"github.com/codyconfer/viewkit/glyph"
)

// StatusFactory builds a status-strip contribution. home/role are the active
// config values; plugins that ignore them may use blank parameters.
type StatusFactory func(home, role string) glyph.StatusContribution

var (
	statusMu        sync.RWMutex
	statusBy        = map[string]StatusFactory{}
	pluginEnabledFn func(id string) bool
)

// SetPluginEnabledFunc wires host enablement checks for [CollectStatusContributions].
// Stock munin calls this from internal/plugin init; overlays rarely need it.
func SetPluginEnabledFunc(fn func(id string) bool) {
	pluginEnabledFn = fn
}

// RegisterStatusContribution registers a plugin status-strip contribution.
// Idempotent for the same plugin id (init + tests). Call alongside Descriptor.
func RegisterStatusContribution(pluginID string, f StatusFactory) {
	if pluginID == "" || f == nil {
		panic("plugin: RegisterStatusContribution requires plugin id and factory")
	}
	statusMu.Lock()
	defer statusMu.Unlock()
	if _, ok := statusBy[pluginID]; ok {
		return
	}
	statusBy[pluginID] = f
}

// CollectStatusContributions returns contributions for enabled plugins,
// sorted by plugin id for stable strip order.
func CollectStatusContributions(home, role string) []glyph.StatusContribution {
	statusMu.RLock()
	ids := make([]string, 0, len(statusBy))
	for id := range statusBy {
		ids = append(ids, id)
	}
	statusMu.RUnlock()
	sort.Strings(ids)

	out := make([]glyph.StatusContribution, 0, len(ids))
	for _, id := range ids {
		if pluginEnabledFn != nil && !pluginEnabledFn(id) {
			continue
		}
		statusMu.RLock()
		f := statusBy[id]
		statusMu.RUnlock()
		if f == nil {
			continue
		}
		out = append(out, f(home, role))
	}
	return out
}
