package plugin

import (
	"github.com/codyconfer/viewkit/deck"
)

// RegisterView registers a viewkit/deck view and a KindView companion
// descriptor (Ref = viewID) under parentID.
func RegisterView(parentID, viewID string, ctor func() deck.View) {
	if parentID == "" || viewID == "" || ctor == nil {
		panic("plugin: RegisterView requires parentID, viewID, and ctor")
	}
	if _, ok := ByKind(KindView, viewID); ok {
		return
	}
	deck.RegisterView(viewID, ctor)
	Register(Descriptor{
		ID:     parentID + "/view/" + viewID,
		Kind:   KindView,
		Ref:    viewID,
		Parent: parentID,
	})
}

// HasView reports whether viewID is registered in the deck view registry.
func HasView(viewID string) bool {
	_, ok := deck.LookupView(viewID)
	return ok
}
