package plugin

import (
	pub "github.com/codyconfer/munin/plugin"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/filter"
	"github.com/codyconfer/munin/internal/signals"
)

func init() {
	config.ExternalFilter = lookupExternalFilter
	filter.ExternalEngine = lookupExternalEngine
}

func lookupExternalFilter(name string) (filter.Filter, bool) {
	f, ok := pub.LookupFilter(name)
	if !ok {
		return filter.Filter{}, false
	}
	return toInternalFilter(f), true
}

func lookupExternalEngine(name string) (func([]signals.Item) []signals.Item, bool) {
	fn, ok := pub.LookupFilterEngine(name)
	if !ok || fn == nil {
		return nil, false
	}
	return fn, true
}

// FilterFunc is a custom KindFilter engine (re-export of public SDK).
type FilterFunc = pub.FilterFunc

// NamedFilter is a YAML-compatible named rule set (re-export of public SDK).
type NamedFilter = pub.NamedFilter

// FilterRule is one include/exclude regex rule (re-export of public SDK).
type FilterRule = pub.FilterRule

// RegisterFilter registers a named filter contribution and a KindFilter
// companion (Ref = filter name) under parentID.
// Prefer github.com/codyconfer/munin/plugin.RegisterFilter from overlays.
func RegisterFilter(parentID string, f filter.Filter) {
	pub.RegisterFilter(parentID, toPublicFilter(f))
}

// RegisterFilterEngine registers a KindFilter backed by Go logic.
// Prefer github.com/codyconfer/munin/plugin.RegisterFilterEngine from overlays.
func RegisterFilterEngine(parentID, name string, fn FilterFunc) {
	pub.RegisterFilterEngine(parentID, name, fn)
}

// LookupFilter returns a plugin-contributed filter by name.
func LookupFilter(name string) (filter.Filter, bool) {
	f, ok := pub.LookupFilter(name)
	if !ok {
		return filter.Filter{}, false
	}
	return toInternalFilter(f), true
}

// LookupFilterEngine returns a custom filter engine by name.
func LookupFilterEngine(name string) (FilterFunc, bool) {
	return pub.LookupFilterEngine(name)
}

// HasFilter reports whether name is a registered plugin filter contribution.
func HasFilter(name string) bool { return pub.HasFilter(name) }

// HasFilterEngine reports whether name has a Go filter engine.
func HasFilterEngine(name string) bool { return pub.HasFilterEngine(name) }

// FilterNames returns registered plugin filter names sorted.
func FilterNames() []string { return pub.FilterNames() }

func toPublicFilter(f filter.Filter) pub.NamedFilter {
	out := pub.NamedFilter{Name: f.Name}
	for _, r := range f.Rules {
		out.Rules = append(out.Rules, pub.FilterRule{
			Field: r.Field, Include: r.Include, Exclude: r.Exclude,
		})
	}
	return out
}

func toInternalFilter(f pub.NamedFilter) filter.Filter {
	out := filter.Filter{Name: f.Name}
	for _, r := range f.Rules {
		out.Rules = append(out.Rules, filter.Rule{
			Field: r.Field, Include: r.Include, Exclude: r.Exclude,
		})
	}
	return out
}
