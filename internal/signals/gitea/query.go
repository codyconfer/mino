package gitea

import (
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/codyconfer/mino/internal/errs"
)

const viewerAlias = "@me"

const (
	searchPath = "/repos/issues/search"
)

var perRepoActorParam = map[string]string{
	"created":   "created_by",
	"assigned":  "assigned_by",
	"mentioned": "mentioned_by",
}

var queryAliases = map[string]string{
	"label":            "labels",
	"milestone":        "milestones",
	"author":           "created",
	"assignee":         "assigned",
	"mentions":         "mentioned",
	"review-requested": "review_requested",
	"org":              "owner",
	"user":             "owner",
}

var queryKeys = map[string]bool{
	"repo":             true,
	"type":             true,
	"state":            true,
	"q":                true,
	"labels":           true,
	"milestones":       true,
	"owner":            true,
	"team":             true,
	"since":            true,
	"before":           true,
	"is":               true,
	"created":          true,
	"assigned":         true,
	"mentioned":        true,
	"review_requested": true,
	"reviewed":         true,
}

var typeValues = map[string]bool{
	"issues": true,
	"pulls":  true,
	"all":    true,
}

var typeAliases = map[string]string{
	"issue": "issues",
	"pr":    "pulls",
	"prs":   "pulls",
	"pull":  "pulls",
}

var stateValues = map[string]bool{
	"open":   true,
	"closed": true,
	"all":    true,
}

var isValues = map[string]bool{
	"open":   true,
	"closed": true,
	"pr":     true,
	"issue":  true,
}

type RepoRef struct {
	Owner string
	Repo  string
}

func (r RepoRef) Zero() bool { return r.Owner == "" || r.Repo == "" }

func (r RepoRef) Slug() string { return r.Owner + "/" + r.Repo }

type Query struct {
	Raw        string
	Repo       RepoRef
	Type       string
	State      string
	Text       string
	Labels     []string
	Milestones []string
	Owner      string
	Team       string
	Since      string
	Before     string
	Flags      map[string]bool
	Logins     map[string]string
}

func ParseQuery(raw string) (Query, error) {
	q := Query{Raw: strings.TrimSpace(raw), Flags: map[string]bool{}, Logins: map[string]string{}}
	var text []string

	for _, tok := range splitTokens(raw) {
		key, rest, found := strings.Cut(tok, ":")
		if !found {
			if v := unquote(tok); v != "" {
				text = append(text, v)
			}
			continue
		}
		key = strings.ToLower(unquote(key))
		if alias, ok := queryAliases[key]; ok {
			key = alias
		}
		if !queryKeys[key] {
			return Query{}, errs.Newf(errs.KindConfig, "gitea: unsupported query qualifier %q", key).
				WithHint("supported qualifiers: %s", strings.Join(sortedKeys(queryKeys), ", "))
		}
		values := splitValues(rest)
		if len(values) == 0 {
			return Query{}, errs.Newf(errs.KindConfig, "gitea: query qualifier %q has no value", key)
		}
		if err := q.apply(key, values, &text); err != nil {
			return Query{}, err
		}
	}

	if q.State == "" {
		q.State = "open"
	}
	q.Text = strings.Join(text, " ")
	if err := q.validate(); err != nil {
		return Query{}, err
	}
	q.scopeActorsToRepo()
	return q, nil
}

func (q *Query) scopeActorsToRepo() {
	if q.CrossRepo() {
		return
	}
	for key := range q.Flags {
		if param, ok := perRepoActorParam[key]; ok {
			q.Logins[param] = viewerAlias
		}
	}
	q.Flags = map[string]bool{}
}

func (q *Query) apply(key string, values []string, text *[]string) error {
	switch key {
	case "repo":
		owner, repo, ok := strings.Cut(values[0], "/")
		if !ok || owner == "" || repo == "" {
			return errs.Newf(errs.KindConfig, "gitea: repo %q is not owner/name", values[0])
		}
		if !q.Repo.Zero() {
			return errs.New(errs.KindConfig, "gitea: only one repo: qualifier is supported").
				WithHint("gitea searches one repository or every repository, never a subset")
		}
		q.Repo = RepoRef{Owner: owner, Repo: repo}
	case "type":
		v := strings.ToLower(values[0])
		if alias, ok := typeAliases[v]; ok {
			v = alias
		}
		if !typeValues[v] {
			return errs.Newf(errs.KindConfig, "gitea: unsupported `type:` value %q", values[0]).
				WithHint("supported values: %s", strings.Join(sortedKeys(typeValues), ", "))
		}
		q.Type = v
	case "state":
		v := strings.ToLower(values[0])
		if !stateValues[v] {
			return errs.Newf(errs.KindConfig, "gitea: unsupported `state:` value %q", values[0]).
				WithHint("supported values: %s", strings.Join(sortedKeys(stateValues), ", "))
		}
		q.State = v
	case "is":
		for _, v := range values {
			switch strings.ToLower(v) {
			case "open", "closed":
				q.State = strings.ToLower(v)
			case "pr":
				q.Type = "pulls"
			case "issue":
				q.Type = "issues"
			default:
				return errs.Newf(errs.KindConfig, "gitea: unsupported `is:` value %q", v).
					WithHint("supported values: %s", strings.Join(sortedKeys(isValues), ", "))
			}
		}
	case "q":
		*text = append(*text, values...)
	case "labels":
		q.Labels = append(q.Labels, values...)
	case "milestones":
		q.Milestones = append(q.Milestones, values...)
	case "owner":
		q.Owner = values[0]
	case "team":
		q.Team = values[0]
	case "since", "before":
		when, err := parseWhen(values[0])
		if err != nil {
			return err
		}
		if key == "since" {
			q.Since = when
		} else {
			q.Before = when
		}
	default:
		return q.applyActor(key, values[0])
	}
	return nil
}

func (q *Query) applyActor(key, value string) error {
	switch strings.ToLower(value) {
	case viewerAlias, "true", "yes":
		q.Flags[key] = true
		return nil
	case "false", "no":
		return nil
	}
	param, ok := perRepoActorParam[key]
	if !ok {
		return errs.Newf(errs.KindConfig, "gitea: %s: takes @me or true, not a login", key).
			WithHint("gitea resolves %s against the token's own user; there is no per-user form of it", key)
	}
	q.Logins[param] = value
	return nil
}

func (q Query) validate() error {
	if q.Repo.Zero() {
		for _, param := range sortedStrings(q.Logins) {
			return errs.Newf(errs.KindConfig, "gitea: %s=%s needs a repository", param, q.Logins[param]).
				WithHint("gitea filters by another user's name only within one repository; add repo:owner/name, " +
					"or use @me to mean the authenticated user")
		}
		return nil
	}
	for _, key := range sortedKeys(q.Flags) {
		if _, ok := perRepoActorParam[key]; ok {
			continue
		}
		return errs.Newf(errs.KindConfig, "gitea: %s: has no per-repository form", key).
			WithHint("drop repo:%s to search across repositories", q.Repo.Slug())
	}
	if q.Owner != "" || q.Team != "" {
		return errs.New(errs.KindConfig, "gitea: owner: and team: cannot be combined with repo:").
			WithHint("repo:%s already names the owner", q.Repo.Slug())
	}
	return nil
}

func (q Query) CrossRepo() bool { return q.Repo.Zero() }

func (q Query) Path() string {
	if q.CrossRepo() {
		return searchPath
	}
	return "/repos/" + url.PathEscape(q.Repo.Owner) + "/" + url.PathEscape(q.Repo.Repo) + "/issues"
}

func (q Query) NeedsViewerLogin() bool {
	if strings.EqualFold(q.Owner, viewerAlias) {
		return true
	}
	for _, login := range q.Logins {
		if strings.EqualFold(login, viewerAlias) {
			return true
		}
	}
	return false
}

func (q Query) WithViewer(login string) Query {
	if login == "" {
		return q
	}
	if strings.EqualFold(q.Owner, viewerAlias) {
		q.Owner = login
	}
	logins := make(map[string]string, len(q.Logins))
	for param, v := range q.Logins {
		if strings.EqualFold(v, viewerAlias) {
			v = login
		}
		logins[param] = v
	}
	q.Logins = logins
	return q
}

func (q Query) Values(limit, page int) url.Values {
	v := url.Values{}
	if q.State != "" {
		v.Set("state", q.State)
	}
	if q.Type != "" && q.Type != "all" {
		v.Set("type", q.Type)
	}
	if q.Text != "" {
		v.Set("q", q.Text)
	}
	if len(q.Labels) > 0 {
		v.Set("labels", strings.Join(q.Labels, ","))
	}
	if len(q.Milestones) > 0 {
		v.Set("milestones", strings.Join(q.Milestones, ","))
	}
	if q.Owner != "" {
		v.Set("owner", q.Owner)
	}
	if q.Team != "" {
		v.Set("team", q.Team)
	}
	if q.Since != "" {
		v.Set("since", q.Since)
	}
	if q.Before != "" {
		v.Set("before", q.Before)
	}
	for _, key := range sortedKeys(q.Flags) {
		v.Set(key, "true")
	}
	for _, param := range sortedStrings(q.Logins) {
		v.Set(param, q.Logins[param])
	}
	if limit > 0 {
		v.Set("limit", strconv.Itoa(limit))
	}
	if page > 0 {
		v.Set("page", strconv.Itoa(page))
	}
	return v
}

func parseWhen(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", errs.New(errs.KindConfig, "gitea: empty time value")
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.UTC().Format(time.RFC3339), nil
	}
	d, err := parseWindow(v)
	if err != nil {
		return "", err
	}
	return time.Now().Add(-d).UTC().Format(time.RFC3339), nil
}

func parseWindow(v string) (time.Duration, error) {
	if strings.HasSuffix(v, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(v, "d"))
		if err == nil && days > 0 {
			return time.Duration(days) * 24 * time.Hour, nil
		}
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, errs.Newf(errs.KindConfig, "gitea: %q is not a time or a window", v).
			WithHint("use an RFC3339 timestamp, or a window like 7d or 24h")
	}
	return d, nil
}

func splitTokens(raw string) []string {
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

func splitValues(raw string) []string {
	var out []string
	var cur strings.Builder
	quoted := false
	flush := func() {
		v := unquote(cur.String())
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

func unquote(v string) string {
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

func sortedStrings(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
