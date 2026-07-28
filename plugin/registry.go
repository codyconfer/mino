package plugin

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

var (
	mu                 sync.RWMutex
	byID               = map[string]Descriptor{}
	signalIndex        = map[string]string{}
	kindIndex          = map[Kind]map[string]string{}
	pendingActionKinds []pendingAction
)

type pendingAction struct {
	signal, name string
	serviceOnly  bool
}

func Register(d Descriptor) {
	mu.Lock()
	defer mu.Unlock()
	registerLocked(d)
	flushPendingActionsLocked()
}

func registerLocked(d Descriptor) {
	if d.ID == "" {
		panic("plugin: empty id")
	}
	if !ValidKind(d.Kind) {
		panic(fmt.Sprintf("plugin: unknown kind %q for %q", d.Kind, d.ID))
	}
	switch d.Kind {
	case KindSignal:
		if d.Signal == "" {
			panic(fmt.Sprintf("plugin: KindSignal %q requires Signal", d.ID))
		}
	default:
		if d.Ref == "" {
			panic(fmt.Sprintf("plugin: kind %q id %q requires Ref", d.Kind, d.ID))
		}
	}
	if _, ok := byID[d.ID]; ok {
		panic(fmt.Sprintf("plugin: duplicate id %q", d.ID))
	}
	key := kindKey(d)
	if kindIndex[d.Kind] == nil {
		kindIndex[d.Kind] = map[string]string{}
	}
	if prev, ok := kindIndex[d.Kind][key]; ok {
		panic(fmt.Sprintf("plugin: duplicate %s ref %q (%s and %s)", d.Kind, key, prev, d.ID))
	}
	byID[d.ID] = d
	kindIndex[d.Kind][key] = d.ID
	if d.Kind == KindSignal && d.Signal != "" {
		signalIndex[d.Signal] = d.ID
	}
}

func kindKey(d Descriptor) string {
	if d.Kind == KindSignal {
		return d.Signal
	}
	return d.Ref
}

func flushPendingActionsLocked() {
	if len(pendingActionKinds) == 0 {
		return
	}
	left := pendingActionKinds[:0]
	for _, p := range pendingActionKinds {
		if !ensureActionKindLocked(p.signal, p.name, p.serviceOnly) {
			left = append(left, p)
		}
	}
	pendingActionKinds = left
}

func ensureActionKindLocked(signal, name string, serviceOnly bool) bool {
	id, ok := signalIndex[signal]
	if !ok {
		return false
	}
	ref := signal + "/" + name
	if kindIndex[KindAction] != nil {
		if _, exists := kindIndex[KindAction][ref]; exists {
			return true
		}
	}
	cid := id + "/action/" + name
	if _, exists := byID[cid]; exists {
		return true
	}
	registerLocked(Descriptor{
		ID:          cid,
		Kind:        KindAction,
		Ref:         ref,
		Parent:      id,
		ServiceOnly: serviceOnly,
	})
	return true
}

func queueActionKindLocked(signal, name string, serviceOnly bool) {
	if ensureActionKindLocked(signal, name, serviceOnly) {
		return
	}
	pendingActionKinds = append(pendingActionKinds, pendingAction{
		signal: signal, name: name, serviceOnly: serviceOnly,
	})
}

const InternalPrefix = "munin."

func IsInternal(id string) bool {
	return strings.HasPrefix(id, InternalPrefix)
}

func lessMenuID(a, b string) bool {
	ai, bi := IsInternal(a), IsInternal(b)
	if ai != bi {
		return ai
	}
	return a < b
}

func All() []Descriptor {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Descriptor, 0, len(byID))
	for _, d := range byID {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func Primaries() []Descriptor {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Descriptor, 0, len(byID))
	for _, d := range byID {
		if d.Parent == "" {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return lessMenuID(out[i].ID, out[j].ID) })
	return out
}

func AllOfKind(kind Kind) []Descriptor {
	mu.RLock()
	defer mu.RUnlock()
	var out []Descriptor
	for _, d := range byID {
		if d.Kind == kind {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func ByKind(kind Kind, ref string) (Descriptor, bool) {
	mu.RLock()
	defer mu.RUnlock()
	id, ok := kindIndex[kind][ref]
	if !ok {
		return Descriptor{}, false
	}
	d, ok := byID[id]
	return d, ok
}

func KnownRefs(kind Kind) map[string]bool {
	mu.RLock()
	defer mu.RUnlock()
	src := kindIndex[kind]
	out := make(map[string]bool, len(src))
	for ref := range src {
		out[ref] = true
	}
	return out
}

func Lookup(id string) (Descriptor, bool) {
	mu.RLock()
	defer mu.RUnlock()
	d, ok := byID[id]
	return d, ok
}

func BySignal(signal string) (Descriptor, bool) {
	mu.RLock()
	defer mu.RUnlock()
	id, ok := signalIndex[signal]
	if !ok {
		return Descriptor{}, false
	}
	d, ok := byID[id]
	return d, ok
}

func KnownSignals() map[string]bool {
	mu.RLock()
	defer mu.RUnlock()
	out := make(map[string]bool, len(signalIndex))
	for s := range signalIndex {
		out[s] = true
	}
	return out
}

func HasCapability(signal string, cap Capability) bool {
	d, ok := BySignal(signal)
	if !ok {
		return false
	}
	for _, c := range d.Capabilities {
		if c == cap {
			return true
		}
	}
	return false
}

func SplitActionRef(ref string) (signal, name string, ok bool) {
	i := strings.IndexByte(ref, '/')
	if i <= 0 || i == len(ref)-1 {
		return "", "", false
	}
	return ref[:i], ref[i+1:], true
}

func OwnerID(d Descriptor) string {
	if d.Parent != "" {
		return d.Parent
	}
	return d.ID
}
