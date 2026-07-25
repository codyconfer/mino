package plugin

import (
	"github.com/codyconfer/viewkit/deck"
)

// RegisterView registers a viewkit/deck view and a KindView companion
// descriptor (Ref = viewID) under parentID. Optional [Option] values
// (e.g. [WithServiceOnly]) configure the companion descriptor.
func RegisterView(parentID, viewID string, ctor func() deck.View, opts ...Option) {
	if parentID == "" || viewID == "" || ctor == nil {
		panic("plugin: RegisterView requires parentID, viewID, and ctor")
	}
	if _, ok := ByKind(KindView, viewID); ok {
		return
	}
	deck.RegisterView(viewID, ctor)
	d := Descriptor{
		ID:     parentID + "/view/" + viewID,
		Kind:   KindView,
		Ref:    viewID,
		Parent: parentID,
	}
	applyOptions(&d, opts)
	Register(d)
}

// HasView reports whether viewID is registered in the deck view registry.
func HasView(viewID string) bool {
	_, ok := deck.LookupView(viewID)
	return ok
}
