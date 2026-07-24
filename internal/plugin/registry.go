package plugin

import (
	"fmt"
	"sort"
	"sync"
)

var (
	mu   sync.RWMutex
	byID = map[string]Descriptor{}
	// signalIndex maps config signal name → plugin id.
	signalIndex = map[string]string{}
)

// Register adds a compile-time plugin descriptor. Panics on duplicate id.
func Register(d Descriptor) {
	if d.ID == "" {
		panic("plugin: empty id")
	}
	mu.Lock()
	defer mu.Unlock()
	if _, ok := byID[d.ID]; ok {
		panic(fmt.Sprintf("plugin: duplicate id %q", d.ID))
	}
	byID[d.ID] = d
	if d.Kind == KindSignal && d.Signal != "" {
		signalIndex[d.Signal] = d.ID
	}
}

// All returns registered descriptors sorted by id.
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

// Lookup returns a descriptor by id.
func Lookup(id string) (Descriptor, bool) {
	mu.RLock()
	defer mu.RUnlock()
	d, ok := byID[id]
	return d, ok
}

// BySignal returns the plugin id registered for a config signal name.
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

// KnownSignals returns signal names that are registered (regardless of enable).
func KnownSignals() map[string]bool {
	mu.RLock()
	defer mu.RUnlock()
	out := make(map[string]bool, len(signalIndex))
	for s := range signalIndex {
		out[s] = true
	}
	return out
}

// HasCapability reports whether the signal's plugin advertises cap.
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
