package gitlab

import (
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/codyconfer/mino/internal/errs"
)

type Kind string

const (
	KindMR       Kind = "mr"
	KindIssue    Kind = "issue"
	KindPipeline Kind = "pipeline"
)

const ViewerAlias = "@me"

type Target struct {
	Scope string
	Path  string
}

type Selector struct {
	Kind   Kind
	Target Target
	Params url.Values

	raw     string
	viewers []string
}

func (s Selector) Raw() string { return s.raw }

func (s Selector) NeedsViewer() bool { return len(s.viewers) > 0 }

func (s Selector) Path() string {
	suffix := "/" + surfacePath(s.Kind)
	switch s.Target.Scope {
	case "project":
		return "projects/" + encodeProject(s.Target.Path) + suffix
	case "group":
		return "groups/" + encodeProject(s.Target.Path) + suffix
	}
	return strings.TrimPrefix(suffix, "/")
}

func surfacePath(k Kind) string {
	switch k {
	case KindIssue:
		return "issues"
	case KindPipeline:
		return "pipelines"
	}
	return "merge_requests"
}

func (s Selector) Query() url.Values { return cloneValues(s.Params) }

// ResolveViewer substitutes the client-side @me. GitLab has no server-side equivalent:
// author_username=@me matches a user literally named "@me" and returns 200 with an empty
// array, so an unresolved alias has to be an error rather than a pass-through.
func (s *Selector) ResolveViewer(login string) error {
	if len(s.viewers) == 0 {
		return nil
	}
	if login == "" {
		return errs.Newf(errs.KindConfig, "gitlab: cannot resolve %s in %q", ViewerAlias, s.raw).
			WithHint("set gitlab.viewer to the username mino should stand in for, or check that the " +
				"credential can read /user")
	}
	for _, key := range s.viewers {
		s.Params.Set(key, login)
	}
	s.viewers = nil
	return nil
}

type termSpec struct {
	param  string
	kinds  []Kind
	values []string
	apply  func(s *Selector, value string) error
}

func onlyMR() []Kind       { return []Kind{KindMR} }
func onlyPipeline() []Kind { return []Kind{KindPipeline} }
func mrAndIssue() []Kind   { return []Kind{KindMR, KindIssue} }

var scopeAliases = map[string]string{
	"assigned": "assigned_to_me",
	"mine":     "assigned_to_me",
	"created":  "created_by_me",
	"authored": "created_by_me",
}

var terms = map[string]termSpec{
	"project":   {apply: setTarget("project")},
	"group":     {apply: setTarget("group")},
	"state":     {param: "state", kinds: mrAndIssue()},
	"scope":     {param: "scope", apply: setScope},
	"author":    {param: "author_username", kinds: mrAndIssue()},
	"assignee":  {param: "assignee_username", kinds: mrAndIssue()},
	"reviewer":  {param: "reviewer_username", kinds: onlyMR()},
	"label":     {param: "labels", kinds: mrAndIssue(), apply: appendCSV("labels")},
	"milestone": {param: "milestone", kinds: mrAndIssue()},
	"search":    {param: "search"},
	"draft":     {param: "wip", kinds: onlyMR(), values: []string{"true", "false"}, apply: setDraft},
	"since":     {param: "updated_after", apply: setSince},
	"sort":      {param: "order_by", values: []string{"updated", "created"}, apply: setSort},
	"status":    {param: "status", kinds: onlyPipeline()},
	"ref":       {param: "ref", kinds: onlyPipeline()},
	"username":  {param: "username", kinds: onlyPipeline()},
}

var stateValues = map[Kind][]string{
	KindMR:    {"opened", "closed", "merged", "locked", "all"},
	KindIssue: {"opened", "closed", "all"},
}

var scopeValues = map[Kind][]string{
	KindMR:       {"assigned_to_me", "created_by_me", "all"},
	KindIssue:    {"assigned_to_me", "created_by_me", "all"},
	KindPipeline: {"running", "pending", "finished", "branches", "tags"},
}

func ParseSelector(raw string) (Selector, error) {
	fields, err := splitTerms(raw)
	if err != nil {
		return Selector{}, err
	}

	s := Selector{Kind: KindMR, Params: url.Values{}, raw: strings.TrimSpace(raw)}
	rest := make([][2]string, 0, len(fields))
	for _, f := range fields {
		key, value, ok := strings.Cut(f, ":")
		if !ok {
			return Selector{}, errs.Newf(errs.KindConfig,
				"gitlab: %q is not a key:value term in selector %q", f, raw).
				WithHint("every term must be one of: %s", strings.Join(knownTerms(), ", "))
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value, err := unquote(strings.TrimSpace(value))
		if err != nil {
			return Selector{}, err
		}
		if key == "kind" {
			k, err := parseKind(value, raw)
			if err != nil {
				return Selector{}, err
			}
			s.Kind = k
			continue
		}
		rest = append(rest, [2]string{key, value})
	}

	for _, kv := range rest {
		if err := s.applyTerm(kv[0], kv[1], raw); err != nil {
			return Selector{}, err
		}
	}
	if err := s.validate(raw); err != nil {
		return Selector{}, err
	}
	return s, nil
}

func (s *Selector) applyTerm(key, value, raw string) error {
	spec, ok := terms[key]
	if !ok {
		return errs.Newf(errs.KindConfig, "gitlab: unknown selector term %q in %q", key, raw).
			WithHint("accepted terms: kind, %s", strings.Join(knownTerms(), ", "))
	}
	if len(spec.kinds) > 0 && !slicesHasKind(spec.kinds, s.Kind) {
		return errs.Newf(errs.KindConfig, "gitlab: %s: applies to %s only, not kind:%s",
			key, kindList(spec.kinds), s.Kind)
	}
	if value == "" {
		return errs.Newf(errs.KindConfig, "gitlab: %s: needs a value in %q", key, raw)
	}
	if len(spec.values) > 0 && !slicesHas(spec.values, value) {
		return errs.Newf(errs.KindConfig, "gitlab: %s: %q is not one of %s",
			key, value, strings.Join(spec.values, ", "))
	}
	if value == ViewerAlias && !viewerCapable(spec.param) {
		return errs.Newf(errs.KindConfig, "gitlab: %s: %s is only meaningful for a username",
			key, ViewerAlias)
	}
	if spec.apply != nil {
		return spec.apply(s, value)
	}
	return s.set(key, spec.param, value)
}

func (s *Selector) set(key, param, value string) error {
	if err := s.checkEnum(key, param, value); err != nil {
		return err
	}
	if value == ViewerAlias {
		s.viewers = append(s.viewers, param)
	}
	s.Params.Set(param, value)
	return nil
}

func (s *Selector) checkEnum(key, param, value string) error {
	var allowed []string
	switch param {
	case "state":
		allowed = stateValues[s.Kind]
	case "scope":
		allowed = scopeValues[s.Kind]
	default:
		return nil
	}
	if len(allowed) == 0 || slicesHas(allowed, value) {
		return nil
	}
	return errs.Newf(errs.KindConfig, "gitlab: %s: %q is not valid for kind:%s (want %s)",
		key, value, s.Kind, strings.Join(allowed, ", "))
}

func viewerCapable(param string) bool {
	switch param {
	case "author_username", "assignee_username", "reviewer_username", "username":
		return true
	}
	return false
}

func setTarget(scope string) func(*Selector, string) error {
	return func(s *Selector, value string) error {
		if s.Target.Scope != "" {
			return errs.Newf(errs.KindConfig, "gitlab: %s: a selector takes one target, already set to %s:%s",
				scope, s.Target.Scope, s.Target.Path)
		}
		s.Target = Target{Scope: scope, Path: value}
		return nil
	}
}

func setScope(s *Selector, value string) error {
	if canonical, ok := scopeAliases[value]; ok && s.Kind != KindPipeline {
		value = canonical
	}
	return s.set("scope", "scope", value)
}

func setDraft(s *Selector, value string) error {
	if value == "true" {
		s.Params.Set("wip", "yes")
	} else {
		s.Params.Set("wip", "no")
	}
	return nil
}

func setSort(s *Selector, value string) error {
	if value == "created" {
		s.Params.Set("order_by", "created_at")
	} else {
		s.Params.Set("order_by", "updated_at")
	}
	s.Params.Set("sort", "desc")
	return nil
}

func setSince(s *Selector, value string) error {
	if t, err := time.Parse("2006-01-02", value); err == nil {
		s.Params.Set("updated_after", t.UTC().Format(time.RFC3339))
		return nil
	}
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		return errs.Newf(errs.KindConfig, "gitlab: since: %q is not a duration or a YYYY-MM-DD date", value)
	}
	s.Params.Set("updated_after", timeNow().Add(-d).UTC().Format(time.RFC3339))
	return nil
}

func appendCSV(param string) func(*Selector, string) error {
	return func(s *Selector, value string) error {
		if cur := s.Params.Get(param); cur != "" {
			s.Params.Set(param, cur+","+value)
			return nil
		}
		s.Params.Set(param, value)
		return nil
	}
}

func (s Selector) validate(raw string) error {
	if s.Kind == KindPipeline && s.Target.Scope != "project" {
		return errs.Newf(errs.KindConfig, "gitlab: kind:pipeline needs a project: in %q", raw).
			WithHint("GitLab has no instance-wide pipelines endpoint; add project:group/name")
	}
	if s.Target.Scope == "group" && s.Kind == KindPipeline {
		return errs.New(errs.KindConfig, "gitlab: pipelines cannot be listed for a group")
	}
	return nil
}

func parseKind(value, raw string) (Kind, error) {
	switch Kind(value) {
	case KindMR, KindIssue, KindPipeline:
		return Kind(value), nil
	}
	switch value {
	case "merge_request", "merge-request", "mrs":
		return KindMR, nil
	case "issues":
		return KindIssue, nil
	case "pipelines":
		return KindPipeline, nil
	}
	return "", errs.Newf(errs.KindConfig, "gitlab: kind: %q is not one of mr, issue, pipeline (in %q)",
		value, raw)
}

func splitTerms(raw string) ([]string, error) {
	var (
		out     []string
		cur     strings.Builder
		inQuote bool
	)
	for _, r := range raw {
		switch {
		case r == '"':
			inQuote = !inQuote
			cur.WriteRune(r)
		case (r == ' ' || r == '\t' || r == '\n') && !inQuote:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if inQuote {
		return nil, errs.Newf(errs.KindConfig, "gitlab: unbalanced quote in selector %q", raw)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out, nil
}

func unquote(v string) (string, error) {
	if len(v) < 2 || !strings.HasPrefix(v, `"`) {
		return v, nil
	}
	s, err := strconv.Unquote(v)
	if err != nil {
		return "", errs.Newf(errs.KindConfig, "gitlab: %s is not a valid quoted value", v)
	}
	return s, nil
}

func knownTerms() []string {
	out := make([]string, 0, len(terms))
	for k := range terms {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func kindList(kinds []Kind) string {
	out := make([]string, 0, len(kinds))
	for _, k := range kinds {
		out = append(out, "kind:"+string(k))
	}
	return strings.Join(out, " or ")
}

func slicesHas(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func slicesHasKind(list []Kind, want Kind) bool {
	for _, k := range list {
		if k == want {
			return true
		}
	}
	return false
}

func SelectorTerms() []string {
	return []string{
		"kind:mr", "kind:issue", "kind:pipeline",
		"state:opened", "state:closed", "state:merged", "state:all",
		"scope:assigned", "scope:created", "scope:all",
		"author:@me", "assignee:@me", "reviewer:@me",
		"draft:false", "label:", "milestone:", "search:",
		"project:", "group:", "ref:", "username:",
		"status:failed", "status:success", "status:running",
		"sort:updated", "sort:created", "since:7d",
	}
}
