package github

import (
	"sort"
	"strings"

	"github.com/codyconfer/munin/internal/errs"
)

const viewerAlias = "@me"

var projectFilterKeys = map[string]bool{
	"status":   true,
	"repo":     true,
	"is":       true,
	"assignee": true,
	"author":   true,
	"label":    true,
	"no":       true,
	"sort":     true,
}

var projectIsValues = map[string]bool{
	"open":   true,
	"closed": true,
	"merged": true,
	"draft":  true,
	"issue":  true,
	"pr":     true,
}

var projectNoValues = map[string]bool{
	"status":   true,
	"assignee": true,
	"label":    true,
}

type projectClause struct {
	key    string
	values []string
	text   string
	negate bool
}

type projectFilter struct {
	clauses     []projectClause
	needsViewer bool
}

func parseProjectFilter(raw string) (projectFilter, error) {
	var pf projectFilter
	for _, tok := range splitProjectTokens(raw) {
		clause, err := parseProjectClause(tok)
		if err != nil {
			return projectFilter{}, err
		}
		if clause.key == "" && clause.text == "" {
			continue
		}
		if clause.key == "assignee" || clause.key == "author" {
			for _, v := range clause.values {
				if strings.EqualFold(v, viewerAlias) {
					pf.needsViewer = true
				}
			}
		}
		pf.clauses = append(pf.clauses, clause)
	}
	return pf, nil
}

func parseProjectClause(tok string) (projectClause, error) {
	clause := projectClause{}
	if strings.HasPrefix(tok, "-") {
		clause.negate = true
		tok = tok[1:]
	}
	key, rest, found := strings.Cut(tok, ":")
	if !found {
		clause.text = unquoteProjectValue(tok)
		return clause, nil
	}
	key = strings.ToLower(strings.TrimSpace(unquoteProjectValue(key)))
	if !projectFilterKeys[key] {
		return projectClause{}, errs.Newf(errs.KindConfig, "github: unsupported project filter qualifier %q", key).
			WithHint("supported qualifiers: %s", strings.Join(sortedKeys(projectFilterKeys), ", "))
	}
	clause.key = key
	clause.values = splitProjectValues(rest)
	if len(clause.values) == 0 {
		return projectClause{}, errs.Newf(errs.KindConfig, "github: project filter qualifier %q has no value", key)
	}
	switch key {
	case "is":
		for _, v := range clause.values {
			if !projectIsValues[strings.ToLower(v)] {
				return projectClause{}, errs.Newf(errs.KindConfig, "github: unsupported `is:` value %q", v).
					WithHint("supported values: %s", strings.Join(sortedKeys(projectIsValues), ", "))
			}
		}
	case "no":
		for _, v := range clause.values {
			if !projectNoValues[strings.ToLower(v)] {
				return projectClause{}, errs.Newf(errs.KindConfig, "github: unsupported `no:` value %q", v).
					WithHint("supported values: %s", strings.Join(sortedKeys(projectNoValues), ", "))
			}
		}
	}
	return clause, nil
}

func splitProjectTokens(raw string) []string {
	var out []string
	var cur strings.Builder
	quoted := false
	for _, r := range raw {
		switch {
		case r == '"':
			quoted = !quoted
			cur.WriteRune(r)
		case (r == ' ' || r == '\t' || r == '\n') && !quoted:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func splitProjectValues(raw string) []string {
	var out []string
	var cur strings.Builder
	quoted := false
	flush := func() {
		v := unquoteProjectValue(cur.String())
		cur.Reset()
		if v != "" {
			out = append(out, v)
		}
	}
	for _, r := range raw {
		switch {
		case r == '"':
			quoted = !quoted
		case r == ',' && !quoted:
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

func unquoteProjectValue(v string) string {
	return strings.TrimSpace(strings.ReplaceAll(v, `"`, ""))
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (c projectClause) local() bool {
	switch c.key {
	case "status", "no":
		return true
	case "is":
		return len(c.values) > 1
	}
	return false
}

func (pf projectFilter) local() projectFilter {
	out := projectFilter{}
	for _, c := range pf.clauses {
		if c.local() {
			out.clauses = append(out.clauses, c)
		}
	}
	return out
}

func (pf projectFilter) searchQuery(projectRef, viewer string) string {
	terms := []string{"project:" + projectRef}
	sorted := false
	for _, c := range pf.clauses {
		if c.local() {
			continue
		}
		if c.key == "sort" {
			sorted = true
		}
		terms = append(terms, c.searchTerms(viewer)...)
	}
	if !sorted {
		terms = append(terms, "sort:updated-desc")
	}
	return strings.Join(terms, " ")
}

func (c projectClause) searchTerms(viewer string) []string {
	prefix := ""
	if c.negate {
		prefix = "-"
	}
	if c.key == "" {
		return []string{prefix + quoteSearchValue(c.text)}
	}
	vals := make([]string, 0, len(c.values))
	for _, v := range c.values {
		if (c.key == "assignee" || c.key == "author") && strings.EqualFold(v, viewerAlias) && viewer != "" {
			v = viewer
		}
		vals = append(vals, quoteSearchValue(v))
	}
	if c.key == "repo" || c.negate {
		out := make([]string, 0, len(vals))
		for _, v := range vals {
			out = append(out, prefix+c.key+":"+v)
		}
		return out
	}
	return []string{c.key + ":" + strings.Join(vals, ",")}
}

func quoteSearchValue(v string) string {
	if strings.ContainsAny(v, " \t") {
		return `"` + v + `"`
	}
	return v
}

func (pf projectFilter) keeps(it projectItem, viewer string) bool {
	for _, c := range pf.clauses {
		if c.matches(it, viewer) == c.negate {
			return false
		}
	}
	return true
}

func (c projectClause) matches(it projectItem, viewer string) bool {
	switch c.key {
	case "":
		return containsFold(it.item.Title, c.text) || containsFold(it.item.Body, c.text)
	case "status":
		return anyFold(c.values, it.status)
	case "repo":
		for _, v := range c.values {
			if strings.EqualFold(v, it.repo) {
				return true
			}
			if !strings.Contains(v, "/") && strings.EqualFold(v, repoName(it.repo)) {
				return true
			}
		}
		return false
	case "author":
		return anyFoldViewer(c.values, viewer, it.author)
	case "assignee":
		for _, v := range c.values {
			if anyFoldViewer([]string{v}, viewer, it.assignees...) {
				return true
			}
		}
		return false
	case "label":
		return anyFold(c.values, it.labels...)
	case "is":
		for _, v := range c.values {
			if it.is(strings.ToLower(v)) {
				return true
			}
		}
		return false
	case "no":
		for _, v := range c.values {
			switch strings.ToLower(v) {
			case "status":
				if it.status == "" {
					return true
				}
			case "assignee":
				if len(it.assignees) == 0 {
					return true
				}
			case "label":
				if len(it.labels) == 0 {
					return true
				}
			}
		}
		return false
	}
	return false
}

func (it projectItem) is(kind string) bool {
	switch kind {
	case "open":
		return strings.EqualFold(it.state, "open")
	case "closed":
		return strings.EqualFold(it.state, "closed") || strings.EqualFold(it.state, "merged")
	case "merged":
		return strings.EqualFold(it.state, "merged")
	case "draft":
		return it.draft
	case "pr":
		return it.kind == "pr"
	case "issue":
		return it.kind == "issue"
	}
	return false
}

func anyFold(values []string, candidates ...string) bool {
	for _, v := range values {
		for _, c := range candidates {
			if strings.EqualFold(v, c) {
				return true
			}
		}
	}
	return false
}

func anyFoldViewer(values []string, viewer string, candidates ...string) bool {
	for _, v := range values {
		if strings.EqualFold(v, viewerAlias) {
			v = viewer
		}
		if v == "" {
			continue
		}
		for _, c := range candidates {
			if strings.EqualFold(v, c) {
				return true
			}
		}
	}
	return false
}

func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

func repoName(slug string) string {
	if _, name, ok := strings.Cut(slug, "/"); ok {
		return name
	}
	return slug
}
