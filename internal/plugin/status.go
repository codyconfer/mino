package plugin

import (
	"github.com/codyconfer/viewkit/glyph"

	pub "github.com/codyconfer/mino/plugin"
)

type StatusFactory = pub.StatusFactory

type StatusEntry = pub.StatusEntry

func RegisterStatusContribution(pluginID string, f StatusFactory) {
	pub.RegisterStatusContribution(pluginID, f)
}

func StatusContributionIDs() []string {
	return pub.StatusContributionIDs()
}

func LookupStatusContribution(pluginID, home, role string) (glyph.StatusContribution, bool) {
	return pub.LookupStatusContribution(pluginID, home, role)
}

func CollectStatusEntries(home, role string) []StatusEntry {
	return pub.CollectStatusEntries(home, role)
}

func CollectStatusContributions(home, role string) []glyph.StatusContribution {
	return pub.CollectStatusContributions(home, role)
}
