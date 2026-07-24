package plugin

// RegisterBuiltins registers stock munin signal plugins with the compile-time
// registry. Call once from init of the host (signals/build).
func RegisterBuiltins() {
	type sig struct {
		id, signal string
		caps       []Capability
	}
	for _, s := range []sig{
		{"munin.demo", "demo", []Capability{CapQuery, CapStream}},
		{"munin.github", "github", []Capability{CapQuery, CapStream}},
		{"munin.calendar", "calendar", []Capability{CapQuery, CapStream}},
		{"munin.gmail", "gmail", []Capability{CapQuery}},
		{"munin.docs", "docs", []Capability{CapQuery}},
		{"munin.drive", "drive", []Capability{CapQuery, CapAction}},
		{"munin.tasks", "tasks", []Capability{CapQuery, CapStream, CapAction}},
		{"munin.slack", "slack", []Capability{CapQuery, CapStream}},
	} {
		if _, ok := Lookup(s.id); ok {
			continue
		}
		Register(Descriptor{
			ID:           s.id,
			Kind:         KindSignal,
			Signal:       s.signal,
			Capabilities: s.caps,
		})
	}
}
