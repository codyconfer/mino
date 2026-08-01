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

var (
	checkpointMu    sync.Mutex
	checkpointOwner string
	checkpointSeen  uint64
)

// noteRegistrationCheckpoint records the last contribution the registry was
// handed. It exists so a host that recovers a panic escaping a plugin's own
// registration code can name the plugin that was mid-registration.
func noteRegistrationCheckpoint(ownerID string) {
	checkpointMu.Lock()
	checkpointSeen++
	if ownerID != "" {
		checkpointOwner = ownerID
	}
	checkpointMu.Unlock()
}

// RegistrationCheckpoint reports the owner id of the most recent contribution
// offered to the registry and how many contributions have been offered so far.
// Compare seen across a registration callback to tell whether the plugin named
// by pluginID was the one that failed.
func RegistrationCheckpoint() (pluginID string, seen uint64) {
	checkpointMu.Lock()
	defer checkpointMu.Unlock()
	return checkpointOwner, checkpointSeen
}

func Register(d Descriptor) { registerDescriptor(d) }

func registerDescriptor(d Descriptor) bool {
	noteRegistrationCheckpoint(OwnerID(d))
	mu.Lock()
	err := registerLocked(d)
	flushPendingActionsLocked()
	mu.Unlock()
	if err != nil {
		noteDiagnostic(Diagnostic{
			PluginID: d.ID,
			Kind:     d.Kind,
			Ref:      kindKey(d),
			Message:  err.Error(),
		})
		return false
	}
	return true
}

func registerLocked(d Descriptor) error {
	if d.ID == "" {
		return fmt.Errorf("descriptor has an empty id; contribution skipped")
	}
	if !ValidKind(d.Kind) {
		return fmt.Errorf("unknown kind %q; contribution skipped", d.Kind)
	}
	switch d.Kind {
	case KindSignal:
		if d.Signal == "" {
			return fmt.Errorf("kind %q requires Signal; contribution skipped", d.Kind)
		}
	default:
		if d.Ref == "" {
			return fmt.Errorf("kind %q requires Ref; contribution skipped", d.Kind)
		}
	}
	if prev, ok := byID[d.ID]; ok {
		return fmt.Errorf("duplicate plugin id %q (already registered as kind %q); contribution skipped", d.ID, prev.Kind)
	}
	key := kindKey(d)
	if prev, ok := kindIndex[d.Kind][key]; ok {
		return fmt.Errorf("%s ref %q is already owned by %q; contribution skipped", d.Kind, key, prev)
	}
	if kindIndex[d.Kind] == nil {
		kindIndex[d.Kind] = map[string]string{}
	}
	byID[d.ID] = d
	kindIndex[d.Kind][key] = d.ID
	if d.Kind == KindSignal && d.Signal != "" {
		signalIndex[d.Signal] = d.ID
	}
	return nil
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
	if err := registerLocked(Descriptor{
		ID:          cid,
		Kind:        KindAction,
		Ref:         ref,
		Parent:      id,
		ServiceOnly: serviceOnly,
	}); err != nil {
		noteDiagnostic(Diagnostic{PluginID: cid, Kind: KindAction, Ref: ref, Message: err.Error()})
	}
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

const InternalPrefix = "mino."

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
	return descriptorHasCapability(d, cap)
}

func descriptorHasCapability(d Descriptor, cap Capability) bool {
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
