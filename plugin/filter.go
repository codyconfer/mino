package plugin

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

type FilterFunc func(items []Item) []Item

type KeywordsFunc func() map[string]string

type FilterRule struct {
	Field   string `yaml:"field" json:"field"`
	Include string `yaml:"include,omitempty" json:"include,omitempty"`
	Exclude string `yaml:"exclude,omitempty" json:"exclude,omitempty"`
}

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

func LookupFilter(name string) (NamedFilter, bool) {
	filterMu.RLock()
	defer filterMu.RUnlock()
	f, ok := namedByName[name]
	if !ok {
		return NamedFilter{}, false
	}
	return cloneNamed(f), true
}

func LookupFilterEngine(name string) (FilterFunc, bool) {
	filterMu.RLock()
	defer filterMu.RUnlock()
	fn, ok := engineByName[name]
	return fn, ok
}

func HasFilter(name string) bool {
	filterMu.RLock()
	defer filterMu.RUnlock()
	_, ok := namedByName[name]
	return ok
}

func HasFilterEngine(name string) bool {
	_, ok := LookupFilterEngine(name)
	return ok
}

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

func HasFilterKeywords(name string) bool {
	filterMu.RLock()
	defer filterMu.RUnlock()
	_, ok := keywordsByName[name]
	return ok
}

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
