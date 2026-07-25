package plugin

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// FilterFunc is a custom KindFilter engine: keep, drop, or reshape items.
// Queries reference the engine by name the same way as YAML filter files.
type FilterFunc func(items []Item) []Item

// KeywordsFunc returns computed keyword values for query param templates.
// Called when a query that references this filter is expanded.
type KeywordsFunc func() map[string]string

// FilterRule is one include/exclude regex rule (YAML filter shape).
type FilterRule struct {
	Field   string `yaml:"field" json:"field"`
	Include string `yaml:"include,omitempty" json:"include,omitempty"`
	Exclude string `yaml:"exclude,omitempty" json:"exclude,omitempty"`
}

// NamedFilter is a YAML-compatible named filter contribution by a plugin.
// Rules filter fetched items; Aliases/Keywords feed query param templates.
type NamedFilter struct {
	Name     string            `yaml:"name" json:"name"`
	Rules    []FilterRule      `yaml:"rules,omitempty" json:"rules,omitempty"`
	Aliases  map[string]string `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	Keywords map[string]string `yaml:"keywords,omitempty" json:"keywords,omitempty"`
}

var (
	filterMu       sync.RWMutex
	namedByName    = map[string]NamedFilter{}
	engineByName   = map[string]FilterFunc{}
	keywordsByName = map[string]KeywordsFunc{}
)

// RegisterFilter registers a KindFilter contribution backed by regex rules
// (same shape as ~/.munin/filters/*.yaml). For non-trivial logic prefer
// [RegisterFilterEngine].
func RegisterFilter(parentID string, f NamedFilter) {
	if parentID == "" {
		panic("plugin: RegisterFilter requires parent plugin id")
	}
	if f.Name == "" {
		panic("plugin: RegisterFilter requires filter name")
	}
	if err := validateNamedFilter(f); err != nil {
		panic(fmt.Sprintf("plugin: RegisterFilter %q: %v", f.Name, err))
	}
	filterMu.Lock()
	if existing, ok := namedByName[f.Name]; ok {
		if !isFilterStub(existing) || engineByName[f.Name] != nil {
			filterMu.Unlock()
			panic(fmt.Sprintf("plugin: duplicate filter %q", f.Name))
		}
	}
	if _, ok := engineByName[f.Name]; ok {
		filterMu.Unlock()
		panic(fmt.Sprintf("plugin: duplicate filter %q", f.Name))
	}
	namedByName[f.Name] = cloneNamed(f)
	filterMu.Unlock()
	registerFilterKind(parentID, f.Name)
}

// RegisterFilterEngine registers a KindFilter contribution backed by Go logic.
// The name is resolvable from query filter refs like a saved YAML filter.
func RegisterFilterEngine(parentID, name string, fn FilterFunc) {
	if parentID == "" {
		panic("plugin: RegisterFilterEngine requires parent plugin id")
	}
	name = strings.TrimSpace(name)
	if name == "" || fn == nil {
		panic("plugin: RegisterFilterEngine requires name and func")
	}
	filterMu.Lock()
	if existing, ok := namedByName[name]; ok {
		if !isFilterStub(existing) || engineByName[name] != nil {
			filterMu.Unlock()
			panic(fmt.Sprintf("plugin: duplicate filter %q", name))
		}
	}
	if _, ok := engineByName[name]; ok {
		filterMu.Unlock()
		panic(fmt.Sprintf("plugin: duplicate filter %q", name))
	}
	engineByName[name] = fn
	namedByName[name] = NamedFilter{Name: name}
	filterMu.Unlock()
	registerFilterKind(parentID, name)
}

// RegisterFilterKeywords registers computed keywords for a KindFilter name.
// The filter may already exist (rules/engine/aliases) or is created as a stub.
// Keywords are merged into query param template context when the filter is referenced.
func RegisterFilterKeywords(parentID, name string, fn KeywordsFunc) {
	if parentID == "" {
		panic("plugin: RegisterFilterKeywords requires parent plugin id")
	}
	name = strings.TrimSpace(name)
	if name == "" || fn == nil {
		panic("plugin: RegisterFilterKeywords requires name and func")
	}
	filterMu.Lock()
	if _, ok := keywordsByName[name]; ok {
		filterMu.Unlock()
		panic(fmt.Sprintf("plugin: duplicate filter keywords %q", name))
	}
	keywordsByName[name] = fn
	if _, ok := namedByName[name]; !ok {
		namedByName[name] = NamedFilter{Name: name}
	}
	filterMu.Unlock()
	registerFilterKind(parentID, name)
}

func registerFilterKind(parentID, name string) {
	if _, ok := ByKind(KindFilter, name); ok {
		return
	}
	cid := parentID + "/filter/" + name
	if _, ok := Lookup(cid); ok {
		return
	}
	Register(Descriptor{
		ID:     cid,
		Kind:   KindFilter,
		Ref:    name,
		Parent: parentID,
	})
}

// LookupFilter returns a plugin-contributed named filter (rules and/or engine stub).
func LookupFilter(name string) (NamedFilter, bool) {
	filterMu.RLock()
	defer filterMu.RUnlock()
	f, ok := namedByName[name]
	if !ok {
		return NamedFilter{}, false
	}
	return cloneNamed(f), true
}

// LookupFilterEngine returns a custom filter engine by name.
func LookupFilterEngine(name string) (FilterFunc, bool) {
	filterMu.RLock()
	defer filterMu.RUnlock()
	fn, ok := engineByName[name]
	return fn, ok
}

// HasFilter reports whether name is a registered plugin filter contribution
// (rules and/or engine).
func HasFilter(name string) bool {
	filterMu.RLock()
	defer filterMu.RUnlock()
	_, ok := namedByName[name]
	return ok
}

// HasFilterEngine reports whether name has a Go filter engine.
func HasFilterEngine(name string) bool {
	_, ok := LookupFilterEngine(name)
	return ok
}

// LookupFilterKeywords returns computed keywords for name, if registered.
func LookupFilterKeywords(name string) (map[string]string, bool) {
	filterMu.RLock()
	fn, ok := keywordsByName[name]
	filterMu.RUnlock()
	if !ok || fn == nil {
		return nil, false
	}
	m := fn()
	if m == nil {
		return map[string]string{}, true
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out, true
}

// HasFilterKeywords reports whether name has a computed keywords func.
func HasFilterKeywords(name string) bool {
	filterMu.RLock()
	defer filterMu.RUnlock()
	_, ok := keywordsByName[name]
	return ok
}

// FilterNames returns registered plugin filter names sorted.
func FilterNames() []string {
	filterMu.RLock()
	defer filterMu.RUnlock()
	out := make([]string, 0, len(namedByName))
	for n := range namedByName {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func cloneNamed(f NamedFilter) NamedFilter {
	out := NamedFilter{Name: f.Name}
	if len(f.Rules) > 0 {
		out.Rules = append([]FilterRule(nil), f.Rules...)
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

// isFilterStub reports a name reserved only for keywords (no rules/aliases yet).
func isFilterStub(f NamedFilter) bool {
	return len(f.Rules) == 0 && len(f.Aliases) == 0 && len(f.Keywords) == 0
}

func validateNamedFilter(f NamedFilter) error {
	for i, r := range f.Rules {
		if r.Include != "" {
			if _, err := regexp.Compile(r.Include); err != nil {
				return fmt.Errorf("rule %d: bad include regex: %w", i, err)
			}
		}
		if r.Exclude != "" {
			if _, err := regexp.Compile(r.Exclude); err != nil {
				return fmt.Errorf("rule %d: bad exclude regex: %w", i, err)
			}
		}
	}
	return nil
}
