package plugin

import (
	"github.com/codyconfer/viewkit/deck"
)

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

func HasView(viewID string) bool {
	_, ok := deck.LookupView(viewID)
	return ok
}
