package plugin

import (
	"github.com/codyconfer/viewkit/deck"

	pub "github.com/codyconfer/munin/plugin"
)

// RegisterView registers a deck view and KindView companion under parentID.
func RegisterView(parentID, viewID string, ctor func() deck.View, opts ...Option) {
	pub.RegisterView(parentID, viewID, ctor, opts...)
}

// HasView reports whether viewID is registered in the deck view registry.
func HasView(viewID string) bool { return pub.HasView(viewID) }
