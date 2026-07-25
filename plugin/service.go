package plugin

// serviceAttachedFn is wired by the host (stock munin sets it from
// internal/plugin) to report whether a live serve/daemon socket is attached.
var serviceAttachedFn func() bool

// SetServiceAttachedFunc wires the host check used by [ServiceAttached] and
// [UIVisible]. Stock munin calls this from internal/plugin init; overlays
// rarely need it. Pass nil to treat the service as detached.
func SetServiceAttachedFunc(fn func() bool) {
	serviceAttachedFn = fn
}

// ServiceAttached reports whether a live munin serve/daemon socket is
// attached for the active home. Returns false when unset or detached.
func ServiceAttached() bool {
	if serviceAttachedFn == nil {
		return false
	}
	return serviceAttachedFn()
}

// UIVisible reports whether d should appear in interactive option lists,
// views, or panels. Service-only contributions are hidden unless a service
// is attached.
func UIVisible(d Descriptor) bool {
	if !d.ServiceOnly {
		return true
	}
	return ServiceAttached()
}

// ViewUIVisible reports whether the KindView contribution for viewID should
// appear in interactive UI. Unknown view ids are treated as visible (not
// gated by this registry).
func ViewUIVisible(viewID string) bool {
	d, ok := ByKind(KindView, viewID)
	if !ok {
		return true
	}
	return UIVisible(d)
}

// ActionUIVisible reports whether the KindAction contribution for
// signal/name should appear in interactive UI. Unknown actions are treated
// as visible.
func ActionUIVisible(signal, name string) bool {
	d, ok := ByKind(KindAction, signal+"/"+name)
	if !ok {
		return true
	}
	return UIVisible(d)
}
