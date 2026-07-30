package build

import "reflect"

// isNilRef reports whether v is an untyped nil interface or a non-nil interface
// wrapping a nil pointer, map, slice, func or channel. `var q *myQuery; return
// q, nil` produces the second form: a plain `v == nil` check lets it through and
// the first method call nil-derefs.
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
