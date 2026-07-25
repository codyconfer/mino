package plugin

import (
	"sort"
	"sync"

	"github.com/codyconfer/viewkit/glyph"
)

// StatusFactory builds a status-strip contribution. home/role are the active
// config values; plugins that ignore them may use blank parameters.
type StatusFactory func(home, role string) glyph.StatusContribution

// StatusEntry pairs a plugin id with its status-strip contribution.
type StatusEntry struct {
	PluginID string
	Contrib  glyph.StatusContribution
}

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

// StatusContributionIDs returns all registered status contribution plugin ids,
// sorted, without enablement filtering.
func StatusContributionIDs() []string {
	statusMu.RLock()
	defer statusMu.RUnlock()
	ids := make([]string, 0, len(statusBy))
	for id := range statusBy {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// LookupStatusContribution builds a contribution without enablement filtering.
func LookupStatusContribution(pluginID, home, role string) (glyph.StatusContribution, bool) {
	statusMu.RLock()
	f := statusBy[pluginID]
	statusMu.RUnlock()
	if f == nil {
		return glyph.StatusContribution{}, false
	}
	return f(home, role), true
}

// CollectStatusEntries returns enabled plugin status contributions with ids,
// sorted by plugin id for stable strip order.
func CollectStatusEntries(home, role string) []StatusEntry {
	ids := StatusContributionIDs()
	out := make([]StatusEntry, 0, len(ids))
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
		out = append(out, StatusEntry{PluginID: id, Contrib: f(home, role)})
	}
	return out
}

// CollectStatusContributions returns contributions for enabled plugins,
// sorted by plugin id for stable strip order.
func CollectStatusContributions(home, role string) []glyph.StatusContribution {
	entries := CollectStatusEntries(home, role)
	out := make([]glyph.StatusContribution, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Contrib)
	}
	return out
}
