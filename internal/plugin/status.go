package plugin

import (
	"github.com/codyconfer/viewkit/glyph"

	pub "github.com/codyconfer/munin/plugin"
)

// StatusFactory builds a status-strip contribution (re-export of public SDK).
type StatusFactory = pub.StatusFactory

// StatusEntry pairs a plugin id with its status-strip contribution.
type StatusEntry = pub.StatusEntry

// RegisterStatusContribution registers a plugin status-strip contribution.
// Prefer github.com/codyconfer/munin/plugin.RegisterStatusContribution from overlays.
func RegisterStatusContribution(pluginID string, f StatusFactory) {
	pub.RegisterStatusContribution(pluginID, f)
}

// StatusContributionIDs returns registered status contribution plugin ids.
func StatusContributionIDs() []string {
	return pub.StatusContributionIDs()
}

// LookupStatusContribution builds a contribution without enablement filtering.
func LookupStatusContribution(pluginID, home, role string) (glyph.StatusContribution, bool) {
	return pub.LookupStatusContribution(pluginID, home, role)
}

// CollectStatusEntries returns enabled plugin status contributions with ids.
func CollectStatusEntries(home, role string) []StatusEntry {
	return pub.CollectStatusEntries(home, role)
}

// CollectStatusContributions returns contributions for enabled plugins.
func CollectStatusContributions(home, role string) []glyph.StatusContribution {
	return pub.CollectStatusContributions(home, role)
}
