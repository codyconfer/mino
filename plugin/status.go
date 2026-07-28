package plugin

import (
	"sort"
	"sync"

	"github.com/codyconfer/viewkit/glyph"
)

type StatusFactory func(home, role string) glyph.StatusContribution

type StatusEntry struct {
	PluginID string
	Contrib  glyph.StatusContribution
}

var (
	statusMu        sync.RWMutex
	statusBy        = map[string]StatusFactory{}
	pluginEnabledFn func(id string) bool
)

func SetPluginEnabledFunc(fn func(id string) bool) {
	pluginEnabledFn = fn
}

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

func LookupStatusContribution(pluginID, home, role string) (glyph.StatusContribution, bool) {
	statusMu.RLock()
	f := statusBy[pluginID]
	statusMu.RUnlock()
	if f == nil {
		return glyph.StatusContribution{}, false
	}
	return f(home, role), true
}

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

func CollectStatusContributions(home, role string) []glyph.StatusContribution {
	entries := CollectStatusEntries(home, role)
	out := make([]glyph.StatusContribution, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Contrib)
	}
	return out
}
