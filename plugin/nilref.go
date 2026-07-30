package plugin

import "reflect"

// isNilRef reports whether v is an unusable nil: either an untyped nil
// interface, or a non-nil interface wrapping a nil pointer, map, slice, func or
// channel. The second form is what `var q *myQuery; return q, nil` produces; a
// plain `v == nil` check passes it through and the first method call panics.
func isNilRef(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func,
		reflect.Chan, reflect.Interface, reflect.UnsafePointer:
		return rv.IsNil()
	default:
		return false
	}
}
