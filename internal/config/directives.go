package config

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	sconfig "github.com/codyconfer/sisyphus/config"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/filter"
)

const (
	DirQueries = "queries"
	DirFilters = "filters"
	DirFlights = "flights"
	DirRoles   = "roles"
)

type Query struct {
	Name    string            `yaml:"name" json:"name"`
	Signal  string            `yaml:"signal" json:"signal"`
	Params  map[string]string `yaml:"params" json:"params"`
	Filters []QueryFilter     `yaml:"filters" json:"filters"`
}

type QueryFilter struct {
	Ref    string
	Inline *filter.Rule
}

func (qf *QueryFilter) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		qf.Ref = n.Value
		return nil
	}
	var r filter.Rule
	if err := n.Decode(&r); err != nil {
		return errs.Wrap(errs.KindConfig, err, "inline filter rule")
	}
	qf.Inline = &r
	return nil
}

func (qf QueryFilter) MarshalYAML() (any, error) {
	if qf.Ref != "" {
		return qf.Ref, nil
	}
	return qf.Inline, nil
}

func (qf *QueryFilter) UnmarshalJSON(b []byte) error {
	if t := bytes.TrimSpace(b); len(t) > 0 && t[0] == '"' {
		var s string
		if err := json.Unmarshal(t, &s); err != nil {
			return err
		}
		qf.Ref = s
		return nil
	}
	var r filter.Rule
	if err := json.Unmarshal(b, &r); err != nil {
		return errs.Wrap(errs.KindConfig, err, "inline filter rule")
	}
	qf.Inline = &r
	return nil
}

func (qf QueryFilter) MarshalJSON() ([]byte, error) {
	if qf.Ref != "" {
		return json.Marshal(qf.Ref)
	}
	return json.Marshal(qf.Inline)
}

type Directives struct {
	Queries map[string]Query
	Filters map[string]filter.Filter
	Flights map[string]Flight
	Roles   map[string]RoleDef
}

func NewDirectives(queries, filters, flights, roles []byte) (*Directives, error) {
	q, err := ParseQueries(queries)
	if err != nil {
		return nil, err
	}
	f, err := ParseFilters(filters)
	if err != nil {
		return nil, err
	}
	fl, err := ParseFlights(flights)
	if err != nil {
		return nil, err
	}
	r, err := ParseRoles(roles)
	if err != nil {
		return nil, err
	}
	return &Directives{Queries: q, Filters: f, Flights: fl, Roles: r}, nil
}

func LoadDirectivesFromFiles(home string) (*Directives, error) {
	q, _, err := sconfig.SerializeDir(filepath.Join(home, DirQueries))
	if err != nil {
		return nil, err
	}
	f, _, err := sconfig.SerializeDir(filepath.Join(home, DirFilters))
	if err != nil {
		return nil, err
	}
	fl, _, err := sconfig.SerializeDir(filepath.Join(home, DirFlights))
	if err != nil {
		return nil, err
	}
	r, _, err := sconfig.SerializeDir(filepath.Join(home, DirRoles))
	if err != nil {
		return nil, err
	}
	return NewDirectives(q, f, fl, r)
}

func parseCollection[T any](blob []byte, kind string, name func(*T) *string, validate func(T) error) (map[string]T, error) {
	c, err := decodeCollection(blob)
	if err != nil {
		return nil, err
	}
	out := make(map[string]T, len(c))
	for _, fn := range sortedKeys(c) {
		var v T
		if err := decodeBytes(fn, []byte(c[fn]), &v); err != nil {
			return nil, err
		}
		np := name(&v)
		if *np == "" {
			*np = baseName(fn)
		}
		if _, dup := out[*np]; dup {
			return nil, errs.Newf(errs.KindConfig, "duplicate %s name %q", kind, *np).WithHint("defined again in %s", fn)
		}
		if validate != nil {
			if err := validate(v); err != nil {
				return nil, err
			}
		}
		out[*np] = v
	}
	return out, nil
}

func ParseQueries(blob []byte) (map[string]Query, error) {
	return parseCollection(blob, "query", func(q *Query) *string { return &q.Name }, nil)
}

func ParseFilters(blob []byte) (map[string]filter.Filter, error) {
	return parseCollection(blob, "filter", func(f *filter.Filter) *string { return &f.Name }, func(f filter.Filter) error {
		_, err := filter.Compile(f)
		return err
	})
}

func ParseFlights(blob []byte) (map[string]Flight, error) {
	return parseCollection(blob, "flight", func(fl *Flight) *string { return &fl.Name }, nil)
}

func ParseRoles(blob []byte) (map[string]RoleDef, error) {
	return parseCollection(blob, "role", func(rd *RoleDef) *string { return &rd.Name }, nil)
}

func (s *Directives) Resolve(q Query) ([]filter.Filter, error) {
	var out []filter.Filter
	inline := filter.Filter{Name: q.Name + " (inline)"}
	for _, qf := range q.Filters {
		switch {
		case qf.Ref != "":
			f, ok := s.Filters[qf.Ref]
			if !ok {
				return nil, errs.Newf(errs.KindConfig, "query %q references unknown filter %q", q.Name, qf.Ref).WithHint("define filter %q or fix the reference", qf.Ref)
			}
			out = append(out, f)
		case qf.Inline != nil:
			inline.Rules = append(inline.Rules, *qf.Inline)
		}
	}
	if len(inline.Rules) > 0 {
		out = append(out, inline)
	}
	return out, nil
}

func (s *Directives) QueryNames() []string  { return sortedKeys(s.Queries) }
func (s *Directives) FilterNames() []string { return sortedKeys(s.Filters) }
func (s *Directives) FlightNames() []string { return sortedKeys(s.Flights) }
func (s *Directives) RoleNames() []string   { return sortedKeys(s.Roles) }

func decodeCollection(blob []byte) (map[string]string, error) {
	if len(blob) == 0 {
		return map[string]string{}, nil
	}
	var c map[string]string
	if err := json.Unmarshal(blob, &c); err != nil {
		return nil, errs.Wrap(errs.KindConfig, err, "decoding collection")
	}
	return c, nil
}

func decodeBytes(name string, data []byte, v any) error {
	if strings.EqualFold(filepath.Ext(name), ".json") {
		if err := json.Unmarshal(data, v); err != nil {
			return errs.Wrapf(errs.KindConfig, err, "parsing %s", name)
		}
		return nil
	}
	if err := yaml.Unmarshal(data, v); err != nil {
		return errs.Wrapf(errs.KindConfig, err, "parsing %s", name)
	}
	return nil
}

func baseName(path string) string {
	b := filepath.Base(path)
	return b[:len(b)-len(filepath.Ext(b))]
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
