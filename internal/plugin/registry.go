package plugin

import pub "github.com/codyconfer/munin/plugin"

// Register adds a compile-time plugin descriptor. Panics on duplicate id.
func Register(d Descriptor) { pub.Register(d) }

// All returns registered descriptors sorted by id (includes companions).
func All() []Descriptor { return pub.All() }

// Primaries returns enableable descriptors (Parent empty).
// Stock munin.* ids are listed first; secondary order is alphabetical by id.
func Primaries() []Descriptor { return pub.Primaries() }

// IsInternal reports whether id is a stock munin plugin (builtin / internal).
func IsInternal(id string) bool { return pub.IsInternal(id) }

// AllOfKind returns descriptors of kind.
func AllOfKind(kind Kind) []Descriptor { return pub.AllOfKind(kind) }

// ByKind returns the descriptor for a Kind+Ref (or Signal for KindSignal).
func ByKind(kind Kind, ref string) (Descriptor, bool) { return pub.ByKind(kind, ref) }

// KnownRefs returns Ref (or Signal) keys registered for kind.
func KnownRefs(kind Kind) map[string]bool { return pub.KnownRefs(kind) }

// Lookup returns a descriptor by id.
func Lookup(id string) (Descriptor, bool) { return pub.Lookup(id) }

// BySignal returns the plugin registered for a config signal name.
func BySignal(signal string) (Descriptor, bool) { return pub.BySignal(signal) }

// KnownSignals returns signal names that are registered (regardless of enable).
func KnownSignals() map[string]bool { return pub.KnownSignals() }

// HasCapability reports whether the signal's plugin advertises cap.
func HasCapability(signal string, cap Capability) bool {
	return pub.HasCapability(signal, cap)
}

// SplitActionRef parses KindAction Ref "signal/name".
func SplitActionRef(ref string) (signal, name string, ok bool) {
	return pub.SplitActionRef(ref)
}

// OwnerID returns the primary plugin id for enablement (Parent or self).
func OwnerID(d Descriptor) string { return pub.OwnerID(d) }
