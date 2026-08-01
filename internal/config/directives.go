package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/filter"
	"github.com/codyconfer/mino/internal/format"
)

const (
	DirQueries    = "queries"
	DirFlights    = "flights"
	DirFormatters = "formatters"
	DirDuckDB     = "duckdb"
	DirReports    = "reports"
	DirLogs       = "logs"
	DirPlugins    = ".plugins"

	KindRoles      = "roles"
	KindFormatters = "formatters"
)

func PluginsDir(home string) string { return filepath.Join(home, DirPlugins) }

type DirectiveType string

const (
	TypeAuto      DirectiveType = ""
	TypeQuery     DirectiveType = "query"
	TypeFilter    DirectiveType = "filter"
	TypeFlight    DirectiveType = "flight"
	TypeRole      DirectiveType = "role"
	TypeFormatter DirectiveType = "formatter"
	TypeDuckDB    DirectiveType = "duckdb"
)

func DirectiveTypes() []DirectiveType {
	return []DirectiveType{TypeQuery, TypeFilter, TypeFlight, TypeRole, TypeFormatter, TypeDuckDB}
}

func typeLabel(k DirectiveType) string {
	switch k {
	case TypeQuery, TypeFilter, TypeFlight, TypeRole, TypeFormatter, TypeDuckDB:
		return string(k)
	}
	return "directive"
}

type Query struct {
	Name      string            `yaml:"name,omitempty" json:"name,omitempty"`
	Type      DirectiveType     `yaml:"type,omitempty" json:"type,omitempty"`
	Title     string            `yaml:"title,omitempty" json:"title,omitempty"`
	Signal    string            `yaml:"signal,omitempty" json:"signal,omitempty"`
	Params    map[string]string `yaml:"params,omitempty" json:"params,omitempty"`
	Filters   []QueryFilter     `yaml:"filters,omitempty" json:"filters,omitempty"`
	Rules     []filter.Rule     `yaml:"rules,omitempty" json:"rules,omitempty"`
	Aliases   map[string]string `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	Keywords  map[string]string `yaml:"keywords,omitempty" json:"keywords,omitempty"`
	Formatter string            `yaml:"formatter,omitempty" json:"formatter,omitempty"`
}

type DuckDBQuery struct {
	Name     string        `yaml:"name,omitempty" json:"name,omitempty"`
	Type     DirectiveType `yaml:"type,omitempty" json:"type,omitempty"`
	Database string        `yaml:"database,omitempty" json:"database,omitempty"`
	SQL      string        `yaml:"sql,omitempty" json:"sql,omitempty"`
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

func (q Query) Kind() DirectiveType {
	if q.Type != TypeAuto {
		return q.Type
	}
	if q.Signal != "" {
		return TypeQuery
	}
	return TypeFilter
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

type directiveDoc struct {
	Name       string            `yaml:"name" json:"name"`
	Type       DirectiveType     `yaml:"type" json:"type"`
	Title      string            `yaml:"title" json:"title"`
	Signal     string            `yaml:"signal" json:"signal"`
	Params     map[string]string `yaml:"params" json:"params"`
	Filters    []QueryFilter     `yaml:"filters" json:"filters"`
	Rules      []filter.Rule     `yaml:"rules" json:"rules"`
	Aliases    map[string]string `yaml:"aliases" json:"aliases"`
	Keywords   map[string]string `yaml:"keywords" json:"keywords"`
	Queries    []string          `yaml:"queries" json:"queries"`
	Flights    []string          `yaml:"flights" json:"flights"`
	Home       string            `yaml:"home" json:"home"`
	Contexts   map[string]string `yaml:"contexts" json:"contexts"`
	Hooks      RoleHooks         `yaml:"hooks" json:"hooks"`
	Status     []RoleStatusBlock `yaml:"status" json:"status"`
	Template   string            `yaml:"template" json:"template"`
	Formatter  string            `yaml:"formatter" json:"formatter"`
	Formatters []string          `yaml:"formatters" json:"formatters"`
	Database   string            `yaml:"database" json:"database"`
	SQL        string            `yaml:"sql" json:"sql"`
}

func (d directiveDoc) hasFilterContent() bool {
	return len(d.Rules) > 0 || len(d.Aliases) > 0 || len(d.Keywords) > 0
}

func (d directiveDoc) hasDirectiveFields() bool {
	return d.Title != "" || d.Signal != "" || d.Home != "" ||
		d.Template != "" || d.Formatter != "" ||
		len(d.Params) > 0 || len(d.Filters) > 0 || len(d.Queries) > 0 ||
		len(d.Flights) > 0 || len(d.Contexts) > 0 || len(d.Status) > 0 ||
		len(d.Formatters) > 0 ||
		d.Database != "" || d.SQL != "" || d.Hooks != (RoleHooks{}) || d.hasFilterContent()
}

func (d directiveDoc) query() Query {
	return Query{
		Name:      d.Name,
		Type:      d.Type,
		Title:     d.Title,
		Signal:    d.Signal,
		Params:    d.Params,
		Filters:   d.Filters,
		Rules:     d.Rules,
		Aliases:   d.Aliases,
		Keywords:  d.Keywords,
		Formatter: d.Formatter,
	}
}

func (d directiveDoc) flight() Flight {
	return Flight{Name: d.Name, Type: d.Type, Queries: d.Queries, Formatter: d.Formatter}
}

func (d directiveDoc) role() RoleDef {
	return RoleDef{
		Name:       d.Name,
		Type:       d.Type,
		Home:       d.Home,
		Flights:    d.Flights,
		Queries:    d.Queries,
		Formatters: d.Formatters,
		Contexts:   d.Contexts,
		Hooks:      d.Hooks,
		Status:     d.Status,
	}
}

func (d directiveDoc) formatter() FormatterDef {
	return FormatterDef{Name: d.Name, Type: d.Type, Title: d.Title, Template: d.Template}
}

func (d directiveDoc) duckDBQuery() DuckDBQuery {
	return DuckDBQuery{Name: d.Name, Type: d.Type, Database: d.Database, SQL: d.SQL}
}

func (d directiveDoc) validate(k DirectiveType) error {
	if k != TypeDuckDB && (d.Database != "" || d.SQL != "") {
		return errs.Newf(errs.KindConfig, "%q is declared `type: %s` but carries DuckDB fields", d.Name, k).
			WithHint("database and sql belong to `type: duckdb` documents")
	}
	if k != TypeFormatter && d.Template != "" {
		return errs.Newf(errs.KindConfig, "%q is declared `type: %s` but carries a `template:`", d.Name, k).
			WithHint("templates belong to `type: formatter` documents")
	}
	if k != TypeRole && len(d.Formatters) > 0 {
		return errs.Newf(errs.KindConfig, "%q is declared `type: %s` but lists `formatters:`", d.Name, k).
			WithHint("only roles scope formatters; use a single `formatter:` to attach one")
	}
	switch k {
	case TypeQuery:
		if d.Signal == "" {
			return errs.Newf(errs.KindConfig, "%q is declared `type: query` but names no signal", d.Name).
				WithHint("add a `signal:`, or drop the `type:` line")
		}
	case TypeFilter:
		if d.Signal != "" {
			return errs.Newf(errs.KindConfig, "%q is declared `type: filter` but names signal %q", d.Name, d.Signal).
				WithHint("filters cannot run; use `type: query` or remove the signal")
		}
		if d.Type == TypeFilter && !d.hasFilterContent() {
			return errs.Newf(errs.KindConfig, "%q is declared `type: filter` but has no rules, aliases, or keywords", d.Name)
		}
	case TypeFlight:
		if d.Signal != "" {
			return errs.Newf(errs.KindConfig, "%q is declared `type: flight` but names signal %q", d.Name, d.Signal).
				WithHint("flights compose queries; give the signal its own `type: query` document")
		}
		if d.hasFilterContent() {
			return errs.Newf(errs.KindConfig, "%q is declared `type: flight` but carries rules, aliases, or keywords", d.Name).
				WithHint("flights only take `queries:`; move filter content to a `type: filter` document")
		}
		if len(d.Queries) == 0 {
			return errs.Newf(errs.KindConfig, "%q is declared `type: flight` but lists no queries", d.Name).
				WithHint("add `queries: [name, ...]`")
		}
	case TypeRole:
		if d.Signal != "" {
			return errs.Newf(errs.KindConfig, "%q is declared `type: role` but names signal %q", d.Name, d.Signal).
				WithHint("roles compose flights and queries; give the signal its own `type: query` document")
		}
		if d.hasFilterContent() {
			return errs.Newf(errs.KindConfig, "%q is declared `type: role` but carries rules, aliases, or keywords", d.Name).
				WithHint("move filter content to a `type: filter` document")
		}
		if d.Formatter != "" {
			return errs.Newf(errs.KindConfig, "%q is declared `type: role` but names a single `formatter:`", d.Name).
				WithHint("roles scope formatters with `formatters: [name, ...]`")
		}
	case TypeFormatter:
		if d.Template == "" {
			return errs.Newf(errs.KindConfig, "%q is declared `type: formatter` but has no template", d.Name).
				WithHint("add a `template:` block")
		}
		if d.Signal != "" {
			return errs.Newf(errs.KindConfig, "%q is declared `type: formatter` but names signal %q", d.Name, d.Signal).
				WithHint("formatters shape results; give the signal its own `type: query` document")
		}
		if d.hasFilterContent() {
			return errs.Newf(errs.KindConfig, "%q is declared `type: formatter` but carries rules, aliases, or keywords", d.Name).
				WithHint("move filter content to a `type: filter` document")
		}
		if len(d.Queries) > 0 || len(d.Flights) > 0 {
			return errs.Newf(errs.KindConfig, "%q is declared `type: formatter` but lists queries or flights", d.Name).
				WithHint("attach a formatter from the other side with `formatter: %s`", d.Name)
		}
	case TypeDuckDB:
		if d.Title != "" || d.Signal != "" || d.Template != "" || d.Formatter != "" ||
			len(d.Params) > 0 || len(d.Filters) > 0 || len(d.Queries) > 0 || len(d.Flights) > 0 ||
			len(d.Contexts) > 0 || len(d.Status) > 0 || len(d.Formatters) > 0 ||
			d.Hooks != (RoleHooks{}) || d.hasFilterContent() {
			return errs.Newf(errs.KindConfig, "%q is declared `type: duckdb` but carries fields from another directive type", d.Name).
				WithHint("DuckDB queries only take name, type, database, and sql")
		}
		if d.Database == "" {
			return errs.Newf(errs.KindConfig, "%q is declared `type: duckdb` but names no database", d.Name).
				WithHint("add `database: audit`, `config`, or `tokens`")
		}
		if !ValidDuckDBDatabase(d.Database) {
			return errs.Newf(errs.KindConfig, "%q names unsupported DuckDB database %q", d.Name, d.Database).
				WithHint("choose one of %s", strings.Join(DuckDBDatabases(), ", "))
		}
		if d.SQL == "" {
			return errs.Newf(errs.KindConfig, "%q is declared `type: duckdb` but has no SQL", d.Name).
				WithHint("add a read-only `sql:` statement")
		}
		if !DuckDBReadOnly(d.SQL) {
			return errs.Newf(errs.KindConfig, "%q has a DuckDB statement that is not read-only", d.Name).
				WithHint("use one select, with, pragma, describe, or show statement")
		}
	default:
		return errs.Newf(errs.KindConfig, "%q has unknown type %q: want one of %v", d.Name, d.Type, DirectiveTypes())
	}
	return nil
}

type sourceKey struct {
	kind DirectiveType
	name string
}

type Directives struct {
	Queries    map[string]Query
	Flights    map[string]Flight
	Roles      map[string]RoleDef
	Formatters map[string]FormatterDef
	DuckDB     map[string]DuckDBQuery

	sources map[sourceKey]string
	docs    map[string]int
}

func newDirectives() *Directives {
	return &Directives{
		Queries:    map[string]Query{},
		Flights:    map[string]Flight{},
		Roles:      map[string]RoleDef{},
		Formatters: map[string]FormatterDef{},
		DuckDB:     map[string]DuckDBQuery{},
		sources:    map[sourceKey]string{},
		docs:       map[string]int{},
	}
}

func (s *Directives) Source(k DirectiveType, name string) string {
	if s == nil {
		return ""
	}
	if k == TypeQuery || k == TypeFilter {
		if rel, ok := s.sources[sourceKey{TypeQuery, name}]; ok {
			return rel
		}
		return s.sources[sourceKey{TypeFilter, name}]
	}
	return s.sources[sourceKey{k, name}]
}

func (s *Directives) DocCount(rel string) int {
	if s == nil {
		return 0
	}
	return s.docs[rel]
}

func NewDirectives(blob []byte) (*Directives, error) {
	d := newDirectives()
	if err := d.absorb(blob); err != nil {
		return nil, err
	}
	return d, nil
}

func ParseDirectives(blob []byte) (*Directives, error) { return NewDirectives(blob) }

func LoadDirectivesFromFiles(home string) (*Directives, error) {
	blob, _, err := SerializeDirectives(home)
	if err != nil {
		return nil, err
	}
	return NewDirectives(blob)
}

func (s *Directives) absorb(blob []byte) error {
	c, err := decodeCollection(blob)
	if err != nil {
		return err
	}
	for _, fn := range sortedKeys(c) {
		docs, err := decodeDocs[directiveDoc](fn, []byte(c[fn]))
		if err != nil {
			return err
		}
		for _, placed := range docs {
			doc, at := placed.val, placed.where()
			if doc.Type == TypeAuto {
				if !doc.hasDirectiveFields() {
					if doc.Name == "" {
						continue
					}
					return errs.Newf(errs.KindConfig, "%s in %s names %q but declares nothing else", at, fn, doc.Name).
						WithHint("add a `type:` line (one of %v) and the fields that go with it", DirectiveTypes())
				}
				return errs.Newf(errs.KindConfig, "%s in %s declares no `type:`", at, fn).
					WithHint("add a `type:` line: one of %v", DirectiveTypes())
			}
			k := doc.Type
			if doc.Name == "" {
				if len(docs) > 1 {
					return errs.Newf(errs.KindConfig, "the %s at %s in %s has no name", typeLabel(k), at, fn).
						WithHint("every document in a multi-document file needs its own `name`")
				}
				doc.Name = baseName(fn)
			}
			if err := doc.validate(k); err != nil {
				return inFile(fn, at, err)
			}
			if err := s.add(k, doc, fn); err != nil {
				return inFile(fn, at, err)
			}
			s.sources[sourceKey{k, doc.Name}] = fn
			s.docs[fn]++
		}
	}
	return nil
}

func inFile(fn, at string, err error) error {
	wrapped := errs.Wrapf(errs.KindOf(err), err, "in %s (%s)", fn, at)
	if hint := errs.Hint(err); hint != "" {
		return wrapped.WithHint("%s", hint)
	}
	return wrapped
}

func (s *Directives) add(k DirectiveType, doc directiveDoc, file string) error {
	dup := func() error {
		e := errs.Newf(errs.KindConfig, "duplicate %s name %q", typeLabel(k), doc.Name)
		if first := s.Source(k, doc.Name); first != "" && first != file {
			return e.WithHint("already defined in %s, and again in %s", first, file)
		}
		return e.WithHint("defined again in %s", file)
	}
	switch k {
	case TypeQuery, TypeFilter:
		if _, exists := s.Queries[doc.Name]; exists {
			return dup()
		}
		q := doc.query()
		if q.HasRules() {
			if _, err := filter.Compile(q.AsFilter()); err != nil {
				return err
			}
		}
		s.Queries[doc.Name] = q
	case TypeFlight:
		if _, exists := s.Flights[doc.Name]; exists {
			return dup()
		}
		s.Flights[doc.Name] = doc.flight()
	case TypeRole:
		if _, exists := s.Roles[doc.Name]; exists {
			return dup()
		}
		s.Roles[doc.Name] = doc.role()
	case TypeFormatter:
		if _, exists := s.Formatters[doc.Name]; exists {
			return dup()
		}
		fd := doc.formatter()
		if _, err := format.Parse(fd.Name, fd.Template); err != nil {
			return err
		}
		s.Formatters[doc.Name] = fd
	case TypeDuckDB:
		if _, exists := s.DuckDB[doc.Name]; exists {
			return dup()
		}
		s.DuckDB[doc.Name] = doc.duckDBQuery()
	}
	return nil
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

func (s *Directives) QueryNames() []string     { return sortedKeys(s.Queries) }
func (s *Directives) FlightNames() []string    { return sortedKeys(s.Flights) }
func (s *Directives) RoleNames() []string      { return sortedKeys(s.Roles) }
func (s *Directives) FormatterNames() []string { return sortedKeys(s.Formatters) }
func (s *Directives) DuckDBNames() []string    { return sortedKeys(s.DuckDB) }

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

type placedDoc[T any] struct {
	val  T
	doc  int
	item int
}

func (p placedDoc[T]) where() string {
	if p.item > 0 {
		return fmt.Sprintf("item %d of document %d", p.item, p.doc)
	}
	return fmt.Sprintf("document %d", p.doc)
}

func placedValues[T any](in []placedDoc[T]) []T {
	out := make([]T, 0, len(in))
	for _, p := range in {
		out = append(out, p.val)
	}
	return out
}

func placeSequence[T any](list []T, doc int) []placedDoc[T] {
	out := make([]placedDoc[T], 0, len(list))
	for i, v := range list {
		out = append(out, placedDoc[T]{val: v, doc: doc, item: i + 1})
	}
	return out
}

func directiveDirs() []string { return []string{DirQueries, DirFlights, DirFormatters, DirDuckDB} }

func normalizeRel(rel string) string {
	return path.Clean(strings.ReplaceAll(filepath.ToSlash(rel), `\`, "/"))
}

func directiveLocation(rel string) bool {
	dir := path.Dir(normalizeRel(rel))
	if dir == "." || dir == "/" {
		return true
	}
	return slices.Contains(directiveDirs(), strings.SplitN(strings.TrimPrefix(dir, "/"), "/", 2)[0])
}

func decodeDocs[T any](name string, data []byte) ([]placedDoc[T], error) {
	if strings.EqualFold(filepath.Ext(name), ".json") {
		return decodeJSONDocs[T](name, data)
	}
	return decodeYAMLDocs[T](name, data)
}

func decodeJSONDocs[T any](name string, data []byte) ([]placedDoc[T], error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, nil
	}
	expected := directiveLocation(name)
	if trimmed[0] == '[' {
		var lenient []T
		if err := json.Unmarshal(trimmed, &lenient); err != nil {
			return nil, errs.Wrapf(errs.KindConfig, err, "parsing %s", name)
		}
		var list []T
		if err := decodeStrictJSON(trimmed, &list); err != nil {
			if !expected && jsonUnknownField(err) != "" && !anyIdentified(lenient) {
				return nil, nil
			}
			return nil, jsonDecodeErr(name, err)
		}
		return placeSequence(list, 1), nil
	}
	var lenient T
	if err := json.Unmarshal(trimmed, &lenient); err != nil {
		return nil, errs.Wrapf(errs.KindConfig, err, "parsing %s", name)
	}
	var v T
	if err := decodeStrictJSON(trimmed, &v); err != nil {
		if !expected && jsonUnknownField(err) != "" && !identified(lenient) {
			return nil, nil
		}
		return nil, jsonDecodeErr(name, err)
	}
	return []placedDoc[T]{{val: v, doc: 1}}, nil
}

func decodeStrictJSON(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func decodeYAMLDocs[T any](name string, data []byte) ([]placedDoc[T], error) {
	shape := yaml.NewDecoder(bytes.NewReader(data))
	strict := yaml.NewDecoder(bytes.NewReader(data))
	strict.KnownFields(true)
	expected := directiveLocation(name)
	var out []placedDoc[T]
	for idx := 1; ; idx++ {
		var doc yaml.Node
		if err := shape.Decode(&doc); err != nil {
			if errors.Is(err, io.EOF) {
				return out, nil
			}
			return nil, decodeErr(name, err)
		}
		vals, err := decodeYAMLDoc[T](name, &doc, strict, expected, idx)
		if err != nil {
			return nil, err
		}
		out = append(out, vals...)
	}
}

func decodeYAMLDoc[T any](name string, n *yaml.Node, strict *yaml.Decoder, expected bool, idx int) ([]placedDoc[T], error) {
	if n.Kind == yaml.DocumentNode {
		if len(n.Content) == 0 {
			return nil, skipYAMLDoc(name, strict)
		}
		n = n.Content[0]
	}
	switch {
	case n.Kind == 0, n.Tag == "!!null":
		return nil, skipYAMLDoc(name, strict)
	case n.Kind == yaml.SequenceNode:
		var list []T
		if err := strict.Decode(&list); err != nil {
			if !expected && onlyUnknownFields(err) && !anyIdentified(list) {
				return nil, nil
			}
			return nil, decodeErr(name, err)
		}
		return placeSequence(list, idx), nil
	}
	var v T
	if err := strict.Decode(&v); err != nil {
		if !expected && onlyUnknownFields(err) && !identified(v) {
			return nil, nil
		}
		return nil, decodeErr(name, err)
	}
	return []placedDoc[T]{{val: v, doc: idx}}, nil
}

type identifiable interface{ identified() bool }

func (d directiveDoc) identified() bool {
	return d.Type != TypeAuto || d.Name != "" || d.hasDirectiveFields()
}

func identified[T any](v T) bool {
	if id, ok := any(v).(identifiable); ok {
		return id.identified()
	}
	return true
}

func anyIdentified[T any](list []T) bool {
	for _, v := range list {
		if identified(v) {
			return true
		}
	}
	return false
}

func onlyUnknownFields(err error) bool {
	var typed *yaml.TypeError
	if !errors.As(err, &typed) || len(typed.Errors) == 0 {
		return false
	}
	for _, m := range typed.Errors {
		if !unknownFieldRe.MatchString(m) {
			return false
		}
	}
	return true
}

func skipYAMLDoc(name string, strict *yaml.Decoder) error {
	var ignored any
	if err := strict.Decode(&ignored); err != nil && !errors.Is(err, io.EOF) {
		return decodeErr(name, err)
	}
	return nil
}

var (
	unknownFieldRe     = regexp.MustCompile(`field (\S+) not found in type \S+`)
	jsonUnknownFieldRe = regexp.MustCompile(`^json: unknown field "([^"]*)"`)
)

func decodeErr(name string, err error) error {
	var typed *yaml.TypeError
	if !errors.As(err, &typed) {
		return errs.Wrapf(errs.KindConfig, err, "parsing %s", name)
	}
	msgs := make([]string, 0, len(typed.Errors))
	var unknown []string
	for _, m := range typed.Errors {
		if found := unknownFieldRe.FindStringSubmatch(m); found != nil {
			unknown = append(unknown, found[1])
			m = unknownFieldRe.ReplaceAllString(m, "unknown field `$1`")
		}
		msgs = append(msgs, m)
	}
	e := errs.Newf(errs.KindConfig, "parsing %s: %s", name, strings.Join(msgs, "; "))
	if len(unknown) > 0 {
		return e.WithHint("%s", unknownFieldHint(name, unknown))
	}
	return e
}

func jsonUnknownField(err error) string {
	if err == nil {
		return ""
	}
	if found := jsonUnknownFieldRe.FindStringSubmatch(err.Error()); found != nil {
		return found[1]
	}
	return ""
}

func jsonDecodeErr(name string, err error) error {
	if key := jsonUnknownField(err); key != "" {
		return errs.Newf(errs.KindConfig, "parsing %s: unknown field `%s`", name, key).
			WithHint("%s", unknownFieldHint(name, []string{key}))
	}
	return errs.Wrapf(errs.KindConfig, err, "parsing %s", name)
}

func unknownFieldHint(name string, keys []string) string {
	seen := map[string]bool{}
	quoted := make([]string, 0, len(keys))
	for _, k := range keys {
		if seen[k] {
			continue
		}
		seen[k] = true
		quoted = append(quoted, "`"+k+"`")
	}
	return fmt.Sprintf("delete %s from %s, or correct the spelling: mino ignores keys it does not know, so the line would silently do nothing; a directive takes %s",
		strings.Join(quoted, ", "), name, strings.Join(directiveFieldNames(), ", "))
}

var directiveFieldNames = sync.OnceValue(func() []string {
	t := reflect.TypeFor[directiveDoc]()
	names := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		tag, _, _ := strings.Cut(t.Field(i).Tag.Get("yaml"), ",")
		if tag != "" && tag != "-" {
			names = append(names, tag)
		}
	}
	sort.Strings(names)
	return names
})

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
