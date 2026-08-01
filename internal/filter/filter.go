package filter

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/signals"
)

type Rule struct {
	Field   string `yaml:"field" json:"field"`
	Include string `yaml:"include,omitempty" json:"include,omitempty"`
	Exclude string `yaml:"exclude,omitempty" json:"exclude,omitempty"`
}

type Filter struct {
	Name     string            `yaml:"name" json:"name"`
	Rules    []Rule            `yaml:"rules,omitempty" json:"rules,omitempty"`
	Aliases  map[string]string `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	Keywords map[string]string `yaml:"keywords,omitempty" json:"keywords,omitempty"`
}

// compiledRule holds a rule's patterns; the lit fields are set only when the
// pattern is a plain literal, in which case strings.Contains replaces the regexp.
type compiledRule struct {
	field      string
	include    *regexp.Regexp
	exclude    *regexp.Regexp
	includeLit string
	excludeLit string
}

type Compiled struct {
	Name   string
	rules  []compiledRule
	engine func([]signals.Item) []signals.Item
}

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
			cr.includeLit = literalOf(r.Include)
		}
		if r.Exclude != "" {
			re, err := regexp.Compile(r.Exclude)
			if err != nil {
				return Compiled{}, errs.Wrapf(errs.KindConfig, err, "filter %q rule %d: bad exclude regex", f.Name, i).WithHint("pattern: %s", r.Exclude)
			}
			cr.exclude = re
			cr.excludeLit = literalOf(r.Exclude)
		}
		c.rules = append(c.rules, cr)
	}
	return c, nil
}

// literalOf returns p when an unanchored regexp search for p is identical to
// strings.Contains, else "". QuoteMeta round-tripping proves p holds none of
// \.+*?()|[]{}^$, so it parses as a bare rune sequence with no anchors or
// flags. U+FFFD is excluded because regexp also matches invalid input bytes
// with it. Callers only pass non-empty p, so "" is an unambiguous "no".
func literalOf(p string) string {
	if regexp.QuoteMeta(p) != p || strings.ContainsRune(p, utf8.RuneError) {
		return ""
	}
	return p
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
		if r.exclude != nil && matches(r.exclude, r.excludeLit, val) {
			return false
		}
		if r.include != nil && !matches(r.include, r.includeLit, val) {
			return false
		}
	}
	return true
}

// matches reports whether val hits re, taking the literal path when lit is set.
func matches(re *regexp.Regexp, lit, val string) bool {
	if lit != "" {
		return strings.Contains(val, lit)
	}
	return re.MatchString(val)
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

func (c Compiled) IsEngine() bool { return c.engine != nil }

func ApplyAll(filters []Compiled, items []signals.Item) []signals.Item {
	for _, c := range filters {
		items = c.Apply(items)
	}
	return items
}

func FieldNames() []string {
	return []string{"title", "subtitle", "body"}
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
