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

// FilterRule is one include/exclude regex rule (YAML filter shape).
type FilterRule struct {
	Field   string `yaml:"field" json:"field"`
	Include string `yaml:"include,omitempty" json:"include,omitempty"`
	Exclude string `yaml:"exclude,omitempty" json:"exclude,omitempty"`
}

// NamedFilter is a YAML-compatible named rule set contributed by a plugin.
type NamedFilter struct {
	Name  string       `yaml:"name" json:"name"`
	Rules []FilterRule `yaml:"rules" json:"rules"`
}

var (
	filterMu     sync.RWMutex
	namedByName  = map[string]NamedFilter{}
	engineByName = map[string]FilterFunc{}
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
	if _, ok := namedByName[f.Name]; ok {
		filterMu.Unlock()
		panic(fmt.Sprintf("plugin: duplicate filter %q", f.Name))
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
	if _, ok := namedByName[name]; ok {
		filterMu.Unlock()
		panic(fmt.Sprintf("plugin: duplicate filter %q", name))
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
	if len(f.Rules) == 0 {
		return out
	}
	out.Rules = append([]FilterRule(nil), f.Rules...)
	return out
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
