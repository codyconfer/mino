package gitea

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/signals"
)

const defaultLimit = 30

const maxResponseBytes = 8 << 20

const (
	scopeIssues = "the read:issue and read:repository scopes"
	scopeRepo   = "the read:repository scope"
	scopeUser   = "the read:user scope"
	scopeNotify = "the read:notification scope"
)

const giteaAuthHintSuffix = "; create a token under Settings -> Applications on your Gitea instance, " +
	"then run `mino login gitea` or set $GITEA_TOKEN"

func giteaAuthHint(scope string) string {
	return "your Gitea credential may lack " + scope + giteaAuthHintSuffix
}

func readBody(resp *http.Response) ([]byte, error) {
	if resp.ContentLength > maxResponseBytes {
		return nil, oversizeBody(resp, resp.ContentLength)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "gitea: reading response body")
	}
	if len(body) > maxResponseBytes {
		return nil, oversizeBody(resp, int64(len(body)))
	}
	return body, nil
}

func oversizeBody(resp *http.Response, n int64) error {
	detail := fmt.Sprintf("response body exceeds the %d MiB limit (%d bytes)", maxResponseBytes>>20, n)
	if resp.StatusCode >= 400 {
		return checkGiteaStatus(resp, []byte(detail), "the required scopes")
	}
	return errs.Newf(errs.KindSignal, "gitea: %s", detail).
		WithHint("%s", webUIHint)
}

const webUIHint = "check that gitea.url points at your instance root; mino appends /api/v1 to it"

func checkGiteaStatus(resp *http.Response, body []byte, missingScope string) error {
	msg := errs.ExcerptBytes(body)
	if resp.StatusCode < 400 {
		return nil
	}
	statusErr := func(kind errs.Kind) *errs.Error {
		return errs.Newf(kind, "gitea api %s: %s", resp.Status, msg)
	}
	if looksLikeHTML(body) {
		return statusErr(errs.KindConfig).WithHint("%s", webUIHint)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return statusErr(errs.KindAuth).WithHint("%s", giteaAuthHint(missingScope))
	case rateLimited(resp):
		e := statusErr(errs.KindSignal)
		if d, ok := retryAfter(resp.Header, time.Now()); ok {
			return e.WithHint("gitea is rate limiting requests; retry after %s", d.Round(time.Second))
		}
		return e.WithHint("gitea is rate limiting requests; retry in a few minutes")
	case resp.StatusCode == http.StatusForbidden:
		return statusErr(errs.KindAuth).WithHint("%s", giteaAuthHint(missingScope))
	case resp.StatusCode == http.StatusNotFound:
		return statusErr(errs.KindSignal).
			WithHint("either the credential lacks %s, or this Gitea version does not expose that endpoint; %s",
				missingScope, webUIHint)
	default:
		return statusErr(errs.KindSignal)
	}
}

func looksLikeHTML(body []byte) bool {
	trimmed := strings.TrimSpace(string(body))
	return strings.HasPrefix(trimmed, "<!") || strings.HasPrefix(trimmed, "<html")
}

type query struct {
	q     Query
	title string
}

type Signal struct {
	queries []query
	backend Backend
	max     int
	detail  Cache
	policy  CachePolicy
	viewer  *viewerCache
}

var defaultQueries = []struct{ q, title string }{
	{"type:pulls state:open created:@me", "Open Pull Requests"},
	{"type:pulls state:open review_requested:@me", "Review Requests"},
}

func DefaultQueries() []string {
	out := make([]string, 0, len(defaultQueries))
	for _, q := range defaultQueries {
		out = append(out, q.q)
	}
	return out
}

func New(queries []string, backend Backend, max int, opts ...Option) (signals.Signal, error) {
	o := applyOptions(opts)
	if max <= 0 {
		max = defaultLimit
	}

	var specs []struct{ q, title string }
	if len(queries) == 0 {
		specs = append(specs, defaultQueries...)
	} else {
		for _, raw := range queries {
			specs = append(specs, struct{ q, title string }{raw, raw})
		}
	}

	qs := make([]query, 0, len(specs))
	for _, spec := range specs {
		parsed, err := ParseQuery(spec.q)
		if err != nil {
			return nil, err
		}
		title := spec.title
		if o.title != "" && len(specs) == 1 {
			title = o.title
		}
		qs = append(qs, query{q: parsed, title: title})
	}

	return &Signal{
		queries: qs,
		backend: backend,
		max:     max,
		detail:  o.detail,
		policy:  o.policy,
		viewer:  &viewerCache{configured: o.viewer},
	}, nil
}

func (s *Signal) Name() string { return "gitea" }

func (s *Signal) Fetch(ctx context.Context) ([]signals.Section, error) {
	sections := make([]signals.Section, 0, len(s.queries))
	for _, q := range s.queries {
		resolved, err := s.resolveViewer(ctx, q.q)
		if err != nil {
			return nil, wrapQuery(q.q.Raw, err)
		}
		res, err := s.backend.SearchIssues(ctx, resolved, s.max)
		if err != nil {
			return nil, wrapQuery(q.q.Raw, err)
		}
		sec, err := mapIssues(res.Body, q.title, res.Total, s.max)
		if err != nil {
			return nil, wrapQuery(q.q.Raw, err)
		}
		sections = append(sections, sec)
	}
	return sections, nil
}

func wrapQuery(raw string, err error) error {
	werr := errs.Wrapf(errs.KindOf(err), err, "gitea: query %q", raw)
	if h := errs.Hint(err); h != "" {
		werr = werr.WithHint("%s", h)
	}
	return werr
}

type issue struct {
	Number    int64  `json:"number"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	HTMLURL   string `json:"html_url"`
	State     string `json:"state"`
	Comments  int    `json:"comments"`
	UpdatedAt string `json:"updated_at"`
	User      *struct {
		Login string `json:"login"`
	} `json:"user"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Assignees []struct {
		Login string `json:"login"`
	} `json:"assignees"`
	Repository *struct {
		FullName string `json:"full_name"`
		Owner    string `json:"owner"`
		Name     string `json:"name"`
	} `json:"repository"`
	PullRequest *struct {
		Merged bool `json:"merged"`
		Draft  bool `json:"draft"`
	} `json:"pull_request"`
}

func (i issue) kind() string {
	if i.PullRequest != nil {
		return "pr"
	}
	return "issue"
}

func (i issue) repoSlug() string {
	if i.Repository == nil {
		return ""
	}
	if i.Repository.FullName != "" {
		return i.Repository.FullName
	}
	if i.Repository.Owner != "" && i.Repository.Name != "" {
		return i.Repository.Owner + "/" + i.Repository.Name
	}
	return i.Repository.Name
}

func mapIssues(raw []byte, title string, total, limit int) (signals.Section, error) {
	var list []issue
	if err := json.Unmarshal(raw, &list); err != nil {
		return signals.Section{}, errs.Wrap(errs.KindSignal, err, "gitea: decoding search response")
	}
	sec := signals.Section{
		Signal: "gitea",
		Title:  title,
		Items:  make([]signals.Item, 0, len(list)),
	}
	for _, it := range list {
		var ts time.Time
		if it.UpdatedAt != "" {
			ts, _ = time.Parse(time.RFC3339, it.UpdatedAt)
		}
		sec.Items = append(sec.Items, signals.Item{
			Kind:      it.kind(),
			Title:     it.Title,
			Subtitle:  it.repoSlug(),
			Body:      it.Body,
			URL:       it.HTMLURL,
			Timestamp: ts,
			Meta:      issueMeta(it),
		})
	}
	sec.Meta = issuesMeta(len(list), total, limit, title)
	return sec, nil
}

func issueMeta(it issue) map[string]string {
	meta := map[string]string{
		"number": strconv.FormatInt(it.Number, 10),
		"state":  strings.ToLower(it.State),
	}
	if it.User != nil && it.User.Login != "" {
		meta["author"] = it.User.Login
	}
	if repo := it.repoSlug(); repo != "" {
		meta["repo"] = repo
	}
	if it.Comments > 0 {
		meta["comments"] = strconv.Itoa(it.Comments)
	}
	if names := labelNames(it); len(names) > 0 {
		meta["labels"] = strings.Join(names, ",")
	}
	if who := assigneeLogins(it); len(who) > 0 {
		meta["assignees"] = strings.Join(who, ",")
	}
	if it.PullRequest != nil {
		if it.PullRequest.Draft {
			meta["draft"] = "true"
		}
		if it.PullRequest.Merged {
			meta["state"] = "merged"
		}
	}
	return meta
}

func labelNames(it issue) []string {
	out := make([]string, 0, len(it.Labels))
	for _, l := range it.Labels {
		if l.Name != "" {
			out = append(out, l.Name)
		}
	}
	return out
}

func assigneeLogins(it issue) []string {
	out := make([]string, 0, len(it.Assignees))
	for _, a := range it.Assignees {
		if a.Login != "" {
			out = append(out, a.Login)
		}
	}
	return out
}

func issuesMeta(shown, total, limit int, title string) map[string]string {
	meta := map[string]string{"shown": strconv.Itoa(shown)}
	switch {
	case total > 0:
		meta["total"] = strconv.Itoa(total)
		if total > shown {
			meta[signals.MetaMore] = strconv.Itoa(total - shown)
		}
	case shown >= limit && limit > 0:
		meta[signals.MetaTruncated] = "true"
		meta["truncated_reason"] = "gitea sent no X-Total-Count; more items may exist beyond the limit"
		log.Debugf("gitea: query %q filled the page with no total reported", title)
	}
	return meta
}
