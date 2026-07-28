package plugin

var serviceAttachedFn func() bool

func SetServiceAttachedFunc(fn func() bool) {
	serviceAttachedFn = fn
}

func ServiceAttached() bool {
	if serviceAttachedFn == nil {
		return false
	}
	return serviceAttachedFn()
}

func UIVisible(d Descriptor) bool {
	if !d.ServiceOnly {
		return true
	}
	return ServiceAttached()
}

func ViewUIVisible(viewID string) bool {
	d, ok := ByKind(KindView, viewID)
	if !ok {
		return true
	}
	return UIVisible(d)
}

func ActionUIVisible(signal, name string) bool {
	d, ok := ByKind(KindAction, signal+"/"+name)
	if !ok {
		return true
	}
	return UIVisible(d)
}
