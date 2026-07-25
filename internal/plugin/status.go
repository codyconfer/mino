package plugin

import (
	"github.com/codyconfer/viewkit/glyph"

	pub "github.com/codyconfer/munin/plugin"
)

// StatusFactory builds a status-strip contribution (re-export of public SDK).
type StatusFactory = pub.StatusFactory

// RegisterStatusContribution registers a plugin status-strip contribution.
// Prefer github.com/codyconfer/munin/plugin.RegisterStatusContribution from overlays.
func RegisterStatusContribution(pluginID string, f StatusFactory) {
	pub.RegisterStatusContribution(pluginID, f)
}

// CollectStatusContributions returns contributions for enabled plugins.
func CollectStatusContributions(home, role string) []glyph.StatusContribution {
	return pub.CollectStatusContributions(home, role)
}
