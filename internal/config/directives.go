package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/filter"
)

const (
	DirQueries = "queries"
	DirFlights = "flights"
	DirLogs    = "logs"
	DirPlugins = ".plugins"
	KindRoles  = "roles"
)

func PluginsDir(home string) string { return filepath.Join(home, DirPlugins) }

type QueryType string

const (
	TypeAuto   QueryType = ""
	TypeQuery  QueryType = "query"
	TypeFilter QueryType = "filter"
)

func QueryTypes() []QueryType { return []QueryType{TypeQuery, TypeFilter} }

type Query struct {
	Name     string            `yaml:"name" json:"name"`
	Type     QueryType         `yaml:"type,omitempty" json:"type,omitempty"`
	Title    string            `yaml:"title,omitempty" json:"title,omitempty"`
	Signal   string            `yaml:"signal,omitempty" json:"signal,omitempty"`
	Params   map[string]string `yaml:"params,omitempty" json:"params,omitempty"`
	Filters  []QueryFilter     `yaml:"filters,omitempty" json:"filters,omitempty"`
	Rules    []filter.Rule     `yaml:"rules,omitempty" json:"rules,omitempty"`
	Aliases  map[string]string `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	Keywords map[string]string `yaml:"keywords,omitempty" json:"keywords,omitempty"`
}

func (q Query) Display() string {
	if q.Title != "" {
		return q.Title
	}
	return q.Name
}

func (q Query) Runnable() bool { return q.Type != TypeFilter && q.Signal != "" }

func (q Query) HasRules() bool {
	return len(q.Rules) > 0 || len(q.Aliases) > 0 || len(q.Keywords) > 0
}

func (q Query) HasFilter() bool { return q.Type != TypeQuery && q.HasRules() }

func (q Query) Kind() QueryType {
	if q.Type != TypeAuto {
		return q.Type
	}
	if q.Signal != "" {
		return TypeQuery
	}
	return TypeFilter
}

func (q Query) validateType() error {
	switch q.Type {
	case TypeAuto:
		return nil
	case TypeQuery:
		if q.Signal == "" {
			return errs.Newf(errs.KindConfig, "%q is declared `type: query` but names no signal", q.Name).
				WithHint("add a `signal:`, or drop the `type:` line")
		}
	case TypeFilter:
		if q.Signal != "" {
			return errs.Newf(errs.KindConfig, "%q is declared `type: filter` but names signal %q", q.Name, q.Signal).
				WithHint("filters cannot run; use `type: query` or remove the signal")
		}
		if !q.HasRules() {
			return errs.Newf(errs.KindConfig, "%q is declared `type: filter` but has no rules, aliases, or keywords", q.Name)
		}
	default:
		return errs.Newf(errs.KindConfig, "%q has unknown type %q: want one of %v", q.Name, q.Type, QueryTypes())
	}
	return nil
}

func (q Query) AsFilter() filter.Filter {
	return filter.Filter{Name: q.Name, Rules: q.Rules, Aliases: q.Aliases, Keywords: q.Keywords}
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
	Flights map[string]Flight
	Roles   map[string]RoleDef
}

func NewDirectives(queries, flights, roles []byte) (*Directives, error) {
	q, err := ParseQueries(queries)
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
	return &Directives{Queries: q, Flights: fl, Roles: r}, nil
}

func LoadDirectivesFromFiles(home string) (*Directives, error) {
	q, _, err := SerializeCollection(home, DirQueries)
	if err != nil {
		return nil, err
	}
	fl, _, err := SerializeCollection(home, DirFlights)
	if err != nil {
		return nil, err
	}
	r, _, err := SerializeCollection(home, KindRoles)
	if err != nil {
		return nil, err
	}
	return NewDirectives(q, fl, r)
}

func parseCollection[T any](blob []byte, kind string, name func(*T) *string, validate func(T) error) (map[string]T, error) {
	c, err := decodeCollection(blob)
	if err != nil {
		return nil, err
	}
	out := make(map[string]T, len(c))
	for _, fn := range sortedKeys(c) {
		docs, err := decodeDocs[T](fn, []byte(c[fn]))
		if err != nil {
			return nil, err
		}
		for i := range docs {
			np := name(&docs[i])
			if *np == "" {
				if len(docs) > 1 {
					return nil, errs.Newf(errs.KindConfig, "%s %d in %s has no name", kind, i+1, fn).
						WithHint("every document in a multi-document file needs its own `name`")
				}
				*np = baseName(fn)
			}
			if _, dup := out[*np]; dup {
				return nil, errs.Newf(errs.KindConfig, "duplicate %s name %q", kind, *np).WithHint("defined again in %s", fn)
			}
			if validate != nil {
				if err := validate(docs[i]); err != nil {
					return nil, err
				}
			}
			out[*np] = docs[i]
		}
	}
	return out, nil
}

func ParseQueries(blob []byte) (map[string]Query, error) {
	return parseCollection(blob, "query", func(q *Query) *string { return &q.Name }, func(q Query) error {
		if err := q.validateType(); err != nil {
			return err
		}
		if !q.HasRules() {
			return nil
		}
		_, err := filter.Compile(q.AsFilter())
		return err
	})
}

func ParseFlights(blob []byte) (map[string]Flight, error) {
	return parseCollection(blob, "flight", func(fl *Flight) *string { return &fl.Name }, nil)
}

func ParseRoles(blob []byte) (map[string]RoleDef, error) {
	return parseCollection(blob, "role", func(rd *RoleDef) *string { return &rd.Name }, nil)
}

var ExternalFilter func(name string) (filter.Filter, bool)

func (s *Directives) Filter(name string) (filter.Filter, bool) {
	q, ok := s.Queries[name]
	if !ok || !q.HasFilter() {
		return filter.Filter{}, false
	}
	return q.AsFilter(), true
}

func (s *Directives) LookupFilter(name string) (filter.Filter, bool) {
	if f, ok := s.Filter(name); ok {
		return f, true
	}
	if ExternalFilter != nil {
		return ExternalFilter(name)
	}
	return filter.Filter{}, false
}

func unknownFilterHint(s *Directives, ref string) string {
	other, ok := s.Queries[ref]
	switch {
	case ok && other.Type == TypeQuery:
		return ref + " is declared `type: query`, so its rules stay private to it"
	case ok:
		return ref + " defines no rules, aliases, or keywords"
	}
	return "define a document named " + ref + " with rules/aliases/keywords, or fix the reference"
}

func (s *Directives) Resolve(q Query) ([]filter.Filter, error) {
	var out []filter.Filter
	own := filter.Filter{
		Name:     q.Name + " (inline)",
		Rules:    append([]filter.Rule(nil), q.Rules...),
		Aliases:  q.Aliases,
		Keywords: q.Keywords,
	}
	for _, qf := range q.Filters {
		switch {
		case qf.Ref != "":
			f, ok := s.LookupFilter(qf.Ref)
			if !ok {
				return nil, errs.Newf(errs.KindConfig, "query %q references unknown filter %q", q.Name, qf.Ref).
					WithHint("%s", unknownFilterHint(s, qf.Ref))
			}
			out = append(out, f)
		case qf.Inline != nil:
			own.Rules = append(own.Rules, *qf.Inline)
		}
	}
	if len(own.Rules) > 0 || len(own.Aliases) > 0 || len(own.Keywords) > 0 {
		out = append(out, own)
	}
	return out, nil
}

func (s *Directives) ExpandParams(q Query) (map[string]string, error) {
	resolved, err := s.Resolve(q)
	if err != nil {
		return nil, err
	}
	return filter.ExpandParams(q.Params, resolved)
}

func (s *Directives) QueryNames() []string  { return sortedKeys(s.Queries) }
func (s *Directives) FlightNames() []string { return sortedKeys(s.Flights) }
func (s *Directives) RoleNames() []string   { return sortedKeys(s.Roles) }

func (s *Directives) RunnableNames() []string {
	return s.filterNames(func(q Query) bool { return q.Runnable() })
}

func (s *Directives) FilterNames() []string {
	return s.filterNames(Query.HasFilter)
}

func (s *Directives) filterNames(keep func(Query) bool) []string {
	var out []string
	for _, n := range sortedKeys(s.Queries) {
		if keep(s.Queries[n]) {
			out = append(out, n)
		}
	}
	return out
}

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

func decodeDocs[T any](name string, data []byte) ([]T, error) {
	if strings.EqualFold(filepath.Ext(name), ".json") {
		return decodeJSONDocs[T](name, data)
	}
	return decodeYAMLDocs[T](name, data)
}

func decodeJSONDocs[T any](name string, data []byte) ([]T, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if trimmed[0] == '[' {
		var list []T
		if err := json.Unmarshal(trimmed, &list); err != nil {
			return nil, errs.Wrapf(errs.KindConfig, err, "parsing %s", name)
		}
		return list, nil
	}
	var v T
	if err := json.Unmarshal(trimmed, &v); err != nil {
		return nil, errs.Wrapf(errs.KindConfig, err, "parsing %s", name)
	}
	return []T{v}, nil
}

func decodeYAMLDocs[T any](name string, data []byte) ([]T, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var out []T
	for {
		var doc yaml.Node
		if err := dec.Decode(&doc); err != nil {
			if errors.Is(err, io.EOF) {
				return out, nil
			}
			return nil, errs.Wrapf(errs.KindConfig, err, "parsing %s", name)
		}
		vals, err := decodeYAMLNode[T](name, &doc)
		if err != nil {
			return nil, err
		}
		out = append(out, vals...)
	}
}

func decodeYAMLNode[T any](name string, n *yaml.Node) ([]T, error) {
	if n.Kind == yaml.DocumentNode {
		if len(n.Content) == 0 {
			return nil, nil
		}
		n = n.Content[0]
	}
	switch {
	case n.Kind == 0, n.Tag == "!!null":
		return nil, nil
	case n.Kind == yaml.SequenceNode:
		var list []T
		if err := n.Decode(&list); err != nil {
			return nil, errs.Wrapf(errs.KindConfig, err, "parsing %s", name)
		}
		return list, nil
	}
	var v T
	if err := n.Decode(&v); err != nil {
		return nil, errs.Wrapf(errs.KindConfig, err, "parsing %s", name)
	}
	return []T{v}, nil
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
