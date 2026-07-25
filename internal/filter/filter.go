package filter

import (
	"regexp"
	"strings"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/signals"
)

type Rule struct {
	Field   string `yaml:"field" json:"field"`
	Include string `yaml:"include,omitempty" json:"include,omitempty"`
	Exclude string `yaml:"exclude,omitempty" json:"exclude,omitempty"`
}

type Filter struct {
	Name  string `yaml:"name" json:"name"`
	Rules []Rule `yaml:"rules" json:"rules"`
}

type compiledRule struct {
	field   string
	include *regexp.Regexp
	exclude *regexp.Regexp
}

type Compiled struct {
	Name   string
	rules  []compiledRule
	engine func([]signals.Item) []signals.Item
}

// ExternalEngine looks up plugin-contributed Go filter engines (KindFilter).
// Wired by plugin.init to plugin.LookupFilterEngine; nil when unused (tests).
var ExternalEngine func(name string) (func([]signals.Item) []signals.Item, bool)

func Compile(f Filter) (Compiled, error) {
	if ExternalEngine != nil {
		if fn, ok := ExternalEngine(f.Name); ok && fn != nil {
			return Compiled{Name: f.Name, engine: fn}, nil
		}
	}
	c := Compiled{Name: f.Name}
	for i, r := range f.Rules {
		cr := compiledRule{field: strings.TrimSpace(r.Field)}
		if cr.field == "" {
			cr.field = "body"
		}
		if r.Include != "" {
			re, err := regexp.Compile(r.Include)
			if err != nil {
				return Compiled{}, errs.Wrapf(errs.KindConfig, err, "filter %q rule %d: bad include regex", f.Name, i).WithHint("pattern: %s", r.Include)
			}
			cr.include = re
		}
		if r.Exclude != "" {
			re, err := regexp.Compile(r.Exclude)
			if err != nil {
				return Compiled{}, errs.Wrapf(errs.KindConfig, err, "filter %q rule %d: bad exclude regex", f.Name, i).WithHint("pattern: %s", r.Exclude)
			}
			cr.exclude = re
		}
		c.rules = append(c.rules, cr)
	}
	return c, nil
}

func CompileAll(filters []Filter) ([]Compiled, error) {
	out := make([]Compiled, 0, len(filters))
	for _, f := range filters {
		c, err := Compile(f)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func (c Compiled) keeps(it signals.Item) bool {
	for _, r := range c.rules {
		val := fieldValue(it, r.field)
		if r.exclude != nil && r.exclude.MatchString(val) {
			return false
		}
		if r.include != nil && !r.include.MatchString(val) {
			return false
		}
	}
	return true
}

func (c Compiled) Apply(items []signals.Item) []signals.Item {
	if c.engine != nil {
		return c.engine(items)
	}
	out := make([]signals.Item, 0, len(items))
	for _, it := range items {
		if c.keeps(it) {
			out = append(out, it)
		}
	}
	return out
}

// IsEngine reports whether this compiled filter is backed by a Go engine.
func (c Compiled) IsEngine() bool { return c.engine != nil }

func ApplyAll(filters []Compiled, items []signals.Item) []signals.Item {
	for _, c := range filters {
		items = c.Apply(items)
	}
	return items
}

func fieldValue(it signals.Item, field string) string {
	switch field {
	case "title":
		return it.Title
	case "subtitle":
		return it.Subtitle
	case "body", "":
		return it.Body
	default:
		if key, ok := strings.CutPrefix(field, "meta."); ok {
			return it.Meta[key]
		}
		return ""
	}
}
