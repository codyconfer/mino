package plugin

import (
	"github.com/codyconfer/viewkit/deck"
)

func RegisterView(parentID, viewID string, ctor func() deck.View, opts ...Option) {
	if parentID == "" || viewID == "" || ctor == nil {
		noteDiagnosticf(parentID, KindView, viewID,
			"RegisterView requires a parent plugin id, a view id, and a non-nil ctor (got id %q, ctor nil=%v); view skipped",
			viewID, ctor == nil)
		return
	}
	if prev, ok := ByKind(KindView, viewID); ok {
		if prev.Parent != parentID {
			noteDiagnosticf(parentID, KindView, viewID,
				"view %q is already owned by %q; later view skipped", viewID, prev.ID)
		}
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
