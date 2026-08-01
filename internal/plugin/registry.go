package plugin

import pub "github.com/codyconfer/mino/plugin"

func Register(d Descriptor) { pub.Register(d) }

func All() []Descriptor { return pub.All() }

func Primaries() []Descriptor { return pub.Primaries() }

func IsInternal(id string) bool { return pub.IsInternal(id) }

func AllOfKind(kind Kind) []Descriptor { return pub.AllOfKind(kind) }

func ByKind(kind Kind, ref string) (Descriptor, bool) { return pub.ByKind(kind, ref) }

func KnownRefs(kind Kind) map[string]bool { return pub.KnownRefs(kind) }

func Lookup(id string) (Descriptor, bool) { return pub.Lookup(id) }

func BySignal(signal string) (Descriptor, bool) { return pub.BySignal(signal) }

func KnownSignals() map[string]bool { return pub.KnownSignals() }

func HasCapability(signal string, cap Capability) bool {
	return pub.HasCapability(signal, cap)
}

func SplitActionRef(ref string) (signal, name string, ok bool) {
	return pub.SplitActionRef(ref)
}

func OwnerID(d Descriptor) string { return pub.OwnerID(d) }
