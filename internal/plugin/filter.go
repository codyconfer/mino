package plugin

import (
	pub "github.com/codyconfer/mino/plugin"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/filter"
	"github.com/codyconfer/mino/internal/signals"
)

func init() {
	config.ExternalFilter = lookupExternalFilter
	filter.ExternalEngine = lookupExternalEngine
	filter.ExternalKeywords = lookupExternalKeywords
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

func lookupExternalKeywords(name string) (map[string]string, bool) {
	return pub.LookupFilterKeywords(name)
}

type FilterFunc = pub.FilterFunc

type KeywordsFunc = pub.KeywordsFunc

type NamedFilter = pub.NamedFilter

type FilterRule = pub.FilterRule

func RegisterFilter(parentID string, f filter.Filter) {
	pub.RegisterFilter(parentID, toPublicFilter(f))
}

func RegisterFilterEngine(parentID, name string, fn FilterFunc) {
	pub.RegisterFilterEngine(parentID, name, fn)
}

func RegisterFilterKeywords(parentID, name string, fn KeywordsFunc) {
	pub.RegisterFilterKeywords(parentID, name, fn)
}

func LookupFilter(name string) (filter.Filter, bool) {
	f, ok := pub.LookupFilter(name)
	if !ok {
		return filter.Filter{}, false
	}
	return toInternalFilter(f), true
}

func LookupFilterEngine(name string) (FilterFunc, bool) {
	return pub.LookupFilterEngine(name)
}

func LookupFilterKeywords(name string) (map[string]string, bool) {
	return pub.LookupFilterKeywords(name)
}

func HasFilter(name string) bool { return pub.HasFilter(name) }

func HasFilterEngine(name string) bool { return pub.HasFilterEngine(name) }

func HasFilterKeywords(name string) bool { return pub.HasFilterKeywords(name) }

func FilterNames() []string { return pub.FilterNames() }

func toPublicFilter(f filter.Filter) pub.NamedFilter {
	out := pub.NamedFilter{Name: f.Name}
	for _, r := range f.Rules {
		out.Rules = append(out.Rules, pub.FilterRule{
			Field: r.Field, Include: r.Include, Exclude: r.Exclude,
		})
	}
	if len(f.Aliases) > 0 {
		out.Aliases = make(map[string]string, len(f.Aliases))
		for k, v := range f.Aliases {
			out.Aliases[k] = v
		}
	}
	if len(f.Keywords) > 0 {
		out.Keywords = make(map[string]string, len(f.Keywords))
		for k, v := range f.Keywords {
			out.Keywords[k] = v
		}
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
	if len(f.Aliases) > 0 {
		out.Aliases = make(map[string]string, len(f.Aliases))
		for k, v := range f.Aliases {
			out.Aliases[k] = v
		}
	}
	if len(f.Keywords) > 0 {
		out.Keywords = make(map[string]string, len(f.Keywords))
		for k, v := range f.Keywords {
			out.Keywords[k] = v
		}
	}
	return out
}
