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
	statusMu sync.RWMutex
	statusBy = map[string]StatusFactory{}

	enabledMu       sync.RWMutex
	pluginEnabledFn func(id string) bool
)

func SetPluginEnabledFunc(fn func(id string) bool) {
	enabledMu.Lock()
	pluginEnabledFn = fn
	enabledMu.Unlock()
}

// pluginEnabled reports whether the host considers a contribution id active.
// With no host wired in (plain SDK use, unit tests) everything is enabled.
func pluginEnabled(id string) bool {
	enabledMu.RLock()
	fn := pluginEnabledFn
	enabledMu.RUnlock()
	return fn == nil || fn(id)
}

func RegisterStatusContribution(pluginID string, f StatusFactory) {
	if pluginID == "" || f == nil {
		noteDiagnosticf(pluginID, "", "",
			"RegisterStatusContribution requires a plugin id and a non-nil factory (id %q, factory nil=%v); status contribution skipped",
			pluginID, f == nil)
		return
	}
	noteRegistrationCheckpoint(pluginID)
	statusMu.Lock()
	_, dup := statusBy[pluginID]
	if !dup {
		statusBy[pluginID] = f
	}
	statusMu.Unlock()
	if dup {
		noteDiagnosticf(pluginID, "", "",
			"a status contribution for %q is already registered; later contribution skipped", pluginID)
	}
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
		if !pluginEnabled(id) {
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
