package plugin

func RegisterBuiltins() {
	type sig struct {
		id, signal string
		caps       []Capability
	}
	for _, s := range []sig{
		{"mino.github", "github", []Capability{CapQuery, CapStream, CapCacheable, CapDetail}},
		{"mino.gitea", "gitea", []Capability{CapQuery, CapStream, CapCacheable, CapDetail}},
		{"mino.gitlab", "gitlab", []Capability{CapQuery, CapStream, CapCacheable, CapDetail}},
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
