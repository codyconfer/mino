package plugin

import (
	"github.com/codyconfer/viewkit/deck"

	pub "github.com/codyconfer/munin/plugin"
)

func RegisterView(parentID, viewID string, ctor func() deck.View, opts ...Option) {
	pub.RegisterView(parentID, viewID, ctor, opts...)
}

func HasView(viewID string) bool { return pub.HasView(viewID) }
