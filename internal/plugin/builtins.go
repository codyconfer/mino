package plugin

import (
	"regexp"
	"strings"

	"github.com/codyconfer/munin/internal/signals"
)

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
	if !HasFilter("demo-no-lorem") {
		RegisterFilterEngine("munin.demo", "demo-no-lorem", demoNoLoremEngine)
	}
}

var loremNoise = regexp.MustCompile(`(?i)\b(lorem|ipsum)\b`)

func demoNoLoremEngine(items []signals.Item) []signals.Item {
	out := make([]signals.Item, 0, len(items))
	for _, it := range items {
		if strings.TrimSpace(it.Title) == "" {
			continue
		}
		if loremNoise.MatchString(it.Body) || loremNoise.MatchString(it.Title) {
			continue
		}
		out = append(out, it)
	}
	return out
}
