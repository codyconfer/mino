package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/signals"
)

const projectScopeHint = "reading projects needs the read:project scope; run `gh auth refresh -s read:project` and retry, or re-run `mino login github`; on a GitHub App, grant read access to organization projects"

const (
	searchPageSize  = 50
	searchMaxPages  = 20
	searchPageBurst = 6
	statusFieldName = "Status"
)

var searchWalks singleflight.Group

type ProjectSpec struct {
	Owner  string
	Number int
	Filter string
	Title  string
	Field  string
	Team   string
}

func (s ProjectSpec) Ref() string { return s.Owner + "/" + strconv.Itoa(s.Number) }

func ParseProjectRef(raw string) (string, int, error) {
	ref := strings.TrimSpace(raw)
	if ref == "" {
		return "", 0, errs.New(errs.KindConfig, "github: empty project reference")
	}
	badRef := func() error {
		return errs.Newf(errs.KindConfig, "github: cannot parse project reference %q", raw).
			WithHint("use `owner/number` (e.g. acme/17) or a project URL")
	}
	if i := strings.Index(ref, "github.com"); i >= 0 {
		ref = strings.TrimPrefix(ref[i+len("github.com"):], "/")
	}
	parts := strings.Split(strings.Trim(ref, "/"), "/")
	if len(parts) >= 4 && (parts[0] == "orgs" || parts[0] == "users") && parts[2] == "projects" {
		n, err := strconv.Atoi(parts[3])
		if err != nil {
			return "", 0, badRef()
		}
		return parts[1], n, nil
	}
	if len(parts) == 2 && parts[0] != "" {
		n, err := strconv.Atoi(parts[1])
		if err != nil {
			return "", 0, badRef()
		}
		return parts[0], n, nil
	}
	return "", 0, badRef()
}

type projectSignal struct {
	spec    ProjectSpec
	backend Backend
	max     int
	cache   RosterCache
	detail  Cache
	policy  CachePolicy
	viewer  string
}

func NewProject(spec ProjectSpec, backend Backend, max int, cache RosterCache, opts ...Option) signals.Signal {
	if spec.Field == "" {
		spec.Field = statusFieldName
	}
	if max <= 0 {
		max = defaultPerPage
	}
	o := applyOptions(opts)
	return &projectSignal{spec: spec, backend: backend, max: max, cache: cache, detail: o.detail, policy: o.policy, viewer: o.viewer}
}

func (p *projectSignal) Name() string { return "github" }

type projectItem struct {
	item            signals.Item
	status          string
	repo            string
	author          string
	assignees       []string
	labels          []string
	state           string
	kind            string
	draft           bool
	lastCommentBy   string
	lastCommentAt   time.Time
	lastCommentTeam bool
}

func (p *projectSignal) Fetch(ctx context.Context) ([]signals.Section, error) {
	pf, err := parseProjectFilter(p.spec.Filter)
	if err != nil {
		return nil, err
	}
	viewer := ""
	if pf.needsViewer {
		viewer = p.viewer
		if viewer == "" {
			viewer, err = p.viewerLogin(ctx)
			if err != nil {
				return nil, err
			}
		}
	}
	var roster *Roster
	if p.spec.Team != "" {
		roster, err = ResolveTeam(ctx, p.backend, p.cache, p.spec.Team, p.policy)
		if err != nil {
			return nil, err
		}
	}
	search := pf.searchQuery(p.spec.Ref(), viewer)
	local := pf.local()

	keeps := func(n searchNode) bool {
		it, ok := n.toProjectItem(p.spec, roster)
		return ok && local.keeps(it, viewer)
	}
	nodes, err := p.searchNodes(ctx, search, keeps)
	if err != nil {
		return nil, err
	}
	items := make([]signals.Item, 0, min(p.max, len(nodes)))
	for _, node := range nodes {
		it, ok := node.toProjectItem(p.spec, roster)
		if !ok || !local.keeps(it, viewer) {
			continue
		}
		items = append(items, it.item)
		if len(items) >= p.max {
			break
		}
	}
	return []signals.Section{p.section(items)}, nil
}

func (p *projectSignal) searchNodes(ctx context.Context, search string, keeps func(searchNode) bool) ([]searchNode, error) {
	key := strings.Join([]string{
		p.spec.Field,
		p.spec.Filter,
		strconv.Itoa(p.max),
		search,
	}, "\x00")
	ch := searchWalks.DoChan(key, func() (any, error) {
		walkCtx, cancel := detachWalkContext(ctx)
		defer cancel()
		return p.walk(walkCtx, search, keeps)
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		if res.Err != nil {
			return nil, res.Err
		}
		nodes, _ := res.Val.([]searchNode)
		if res.Shared {
			log.Debugf("github: reused in-flight search walk for %q (%d nodes)", search, len(nodes))
		}
		return nodes, nil
	}
}

func detachWalkContext(ctx context.Context) (context.Context, context.CancelFunc) {
	detached := context.WithoutCancel(ctx)
	if deadline, ok := ctx.Deadline(); ok {
		return context.WithDeadline(detached, deadline)
	}
	return context.WithCancel(detached)
}

func (p *projectSignal) walk(ctx context.Context, search string, keeps func(searchNode) bool) ([]searchNode, error) {
	first, err := p.page(ctx, search, "")
	if err != nil {
		return nil, err
	}
	nodes := dedupeNodes(append([]searchNode(nil), first.Nodes...))
	kept := countKept(nodes, keeps)
	from, left := first, searchMaxPages-1
	if kept >= p.max || left <= 0 || !first.PageInfo.HasNextPage || first.PageInfo.EndCursor == "" {
		return nodes, nil
	}
	if burst := p.burstPages(first, kept, left); burst > 0 {
		got, last, err := p.burst(ctx, search, burst)
		if err != nil {
			return nil, err
		}
		nodes, from, left = dedupeNodes(append(nodes, got...)), last, left-burst
		kept = countKept(nodes, keeps)
	}
	tail, err := p.crawl(ctx, search, from, left, p.max-kept, keeps)
	if err != nil {
		return nil, err
	}
	return dedupeNodes(append(nodes, tail...)), nil
}

func (p *projectSignal) burstPages(first *searchResult, kept, left int) int {
	if first.PageInfo.EndCursor != searchCursor(searchPageSize) {
		log.Debugf("github: unrecognised search cursor %q; walking pages one at a time", first.PageInfo.EndCursor)
		return 0
	}
	return min(left, pagesNeeded(p.max-kept, kept), pagesFor(first.IssueCount)-1)
}

func pagesNeeded(want, kept int) int {
	if want <= 0 {
		return 0
	}
	if kept <= 0 {
		return searchPageBurst
	}
	return (want + kept - 1) / kept
}

func countKept(nodes []searchNode, keeps func(searchNode) bool) int {
	n := 0
	for _, node := range nodes {
		if keeps(node) {
			n++
		}
	}
	return n
}

func (p *projectSignal) burst(ctx context.Context, search string, pages int) ([]searchNode, *searchResult, error) {
	results := make([]*searchResult, pages)
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(searchPageBurst)
	for i := range results {
		g.Go(func() error {
			res, err := p.page(gctx, search, searchCursor((i+1)*searchPageSize))
			if err != nil {
				return err
			}
			results[i] = res
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, nil, err
	}
	var out []searchNode
	for _, res := range results {
		out = append(out, res.Nodes...)
	}
	return out, results[len(results)-1], nil
}

func (p *projectSignal) crawl(ctx context.Context, search string, from *searchResult, pages, want int, keeps func(searchNode) bool) ([]searchNode, error) {
	var nodes []searchNode
	cursor := from.PageInfo.EndCursor
	for range pages {
		if want <= 0 || !from.PageInfo.HasNextPage || cursor == "" {
			break
		}
		res, err := p.page(ctx, search, cursor)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, res.Nodes...)
		want -= countKept(res.Nodes, keeps)
		from, cursor = res, res.PageInfo.EndCursor
	}
	return nodes, nil
}

func pagesFor(items int) int {
	if items <= 0 {
		return 1
	}
	return (items + searchPageSize - 1) / searchPageSize
}

func searchCursor(offset int) string {
	return base64.StdEncoding.EncodeToString([]byte("cursor:" + strconv.Itoa(offset)))
}

func dedupeNodes(nodes []searchNode) []searchNode {
	seen := make(map[string]bool, len(nodes))
	out := nodes[:0]
	for _, n := range nodes {
		if n.URL != "" {
			if seen[n.URL] {
				continue
			}
			seen[n.URL] = true
		}
		out = append(out, n)
	}
	return out
}

func (p *projectSignal) section(items []signals.Item) signals.Section {
	title := p.spec.Title
	if title == "" {
		title = "project " + p.spec.Ref()
		if p.spec.Filter != "" {
			title += " · " + p.spec.Filter
		}
	}
	return signals.Section{Signal: "github", Title: title, Items: items}
}

type searchResult struct {
	IssueCount int `json:"issueCount"`
	PageInfo   struct {
		HasNextPage bool   `json:"hasNextPage"`
		EndCursor   string `json:"endCursor"`
	} `json:"pageInfo"`
	Nodes []searchNode `json:"nodes"`
}

type searchNode struct {
	TypeName   string `json:"__typename"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	Body       string `json:"body"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
	State      string `json:"state"`
	IsDraft    bool   `json:"isDraft"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
	Author *struct {
		Login string `json:"login"`
	} `json:"author"`
	Comments struct {
		Nodes []struct {
			CreatedAt string `json:"createdAt"`
			Author    *struct {
				Login    string `json:"login"`
				TypeName string `json:"__typename"`
			} `json:"author"`
		} `json:"nodes"`
	} `json:"comments"`
	Assignees struct {
		Nodes []struct {
			Login string `json:"login"`
		} `json:"nodes"`
	} `json:"assignees"`
	Labels struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"labels"`
	ProjectItems struct {
		Nodes []struct {
			Project struct {
				Number int `json:"number"`
				Owner  struct {
					Login string `json:"login"`
				} `json:"owner"`
			} `json:"project"`
			Status *struct {
				Name string `json:"name"`
			} `json:"status"`
		} `json:"nodes"`
	} `json:"projectItems"`
}

func (n searchNode) toProjectItem(spec ProjectSpec, roster *Roster) (projectItem, bool) {
	if n.Title == "" && n.URL == "" {
		return projectItem{}, false
	}
	status, onBoard := n.projectStatus(spec)
	if !onBoard {
		return projectItem{}, false
	}
	it := projectItem{
		status: status,
		repo:   n.Repository.NameWithOwner,
		state:  strings.ToLower(n.State),
		draft:  n.IsDraft,
	}
	if n.TypeName == "PullRequest" {
		it.kind = "pr"
	} else {
		it.kind = "issue"
	}
	if n.Author != nil {
		it.author = n.Author.Login
	}
	for _, a := range n.Assignees.Nodes {
		it.assignees = append(it.assignees, a.Login)
	}
	for _, l := range n.Labels.Nodes {
		it.labels = append(it.labels, l.Name)
	}
	it.lastCommentBy, it.lastCommentAt = lastResponder(n)
	it.lastCommentTeam = roster.Has(it.lastCommentBy)

	ts := parseTime(n.UpdatedAt)
	meta := map[string]string{}
	if it.status != "" {
		meta[strings.ToLower(spec.Field)] = it.status
	}
	if it.author != "" {
		meta["author"] = it.author
	}
	if it.state != "" {
		meta["state"] = it.state
	}
	if len(it.assignees) > 0 {
		meta["assignees"] = strings.Join(it.assignees, ", ")
	}
	if len(it.labels) > 0 {
		meta["labels"] = strings.Join(it.labels, ", ")
	}
	if it.draft {
		meta["draft"] = "true"
	}
	if it.lastCommentBy != "" {
		meta["last_comment_by"] = it.lastCommentBy
		if !it.lastCommentAt.IsZero() {
			meta["last_comment_at"] = it.lastCommentAt.Format(time.RFC3339)
		}
		if roster.Configured() {
			meta["last_comment_team"] = strconv.FormatBool(it.lastCommentTeam)
		}
	}
	it.item = signals.Item{
		Kind:      it.kind,
		Title:     n.Title,
		Subtitle:  projectSubtitle(it),
		Body:      n.Body,
		URL:       n.URL,
		Timestamp: ts,
		Meta:      meta,
	}
	return it, true
}

func (n searchNode) projectStatus(spec ProjectSpec) (string, bool) {
	for _, pi := range n.ProjectItems.Nodes {
		if pi.Project.Number != spec.Number {
			continue
		}
		if login := pi.Project.Owner.Login; login != "" && !strings.EqualFold(login, spec.Owner) {
			continue
		}
		if pi.Status != nil {
			return pi.Status.Name, true
		}
		return "", true
	}
	return "", false
}

func projectSubtitle(it projectItem) string {
	parts := make([]string, 0, 2)
	if it.repo != "" {
		parts = append(parts, it.repo)
	}
	if it.status != "" {
		parts = append(parts, it.status)
	}
	return strings.Join(parts, " · ")
}

type graphQLResponse struct {
	Data struct {
		Search *searchResult `json:"search"`
		Viewer *struct {
			Login string `json:"login"`
		} `json:"viewer"`
		Organization *struct {
			Team *struct {
				Members struct {
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
					Nodes []struct {
						Login string `json:"login"`
					} `json:"nodes"`
				} `json:"members"`
			} `json:"team"`
		} `json:"organization"`
	} `json:"data"`
	graphQLErrors
}

type graphQLErrors struct {
	Errors []struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"errors"`
}

func (r graphQLResponse) err() error { return r.errHint(projectScopeHint) }

func (g graphQLErrors) errHint(fallbackHint string) error {
	if len(g.Errors) == 0 {
		return nil
	}
	msgs := make([]string, 0, len(g.Errors))
	scoped, missing := false, false
	for _, e := range g.Errors {
		// GitHub repeats the same complaint once per offending field.
		if !slices.Contains(msgs, e.Message) {
			msgs = append(msgs, e.Message)
		}
		switch e.Type {
		case "INSUFFICIENT_SCOPES":
			scoped = true
		case "NOT_FOUND":
			missing = true
		}
	}
	joined := strings.Join(msgs, "; ")
	switch {
	case scoped:
		scopes := missingScopes(joined)
		detail := joined
		if len(scopes) > 0 {
			detail = scopeSummary(scopes)
		}
		return errs.Newf(errs.KindAuth, "github: graphql: %s", detail).
			WithHint("%s", scopeHint("", scopes, fallbackHint))
	case missing:
		return errs.Newf(errs.KindUsage, "github: %s", joined)
	}
	return errs.Newf(errs.KindSignal, "github: graphql: %s", joined)
}

func (p *projectSignal) page(ctx context.Context, search, cursor string) (*searchResult, error) {
	vars := map[string]any{"q": search, "field": p.spec.Field}
	if cursor != "" {
		vars["after"] = cursor
	}
	raw, err := p.backend.GraphQL(ctx, projectSearchQuery, vars)
	if err != nil {
		return nil, err
	}
	var resp graphQLResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "github: decoding project search response")
	}
	if err := resp.err(); err != nil {
		return nil, err
	}
	if resp.Data.Search == nil {
		return nil, errs.Newf(errs.KindSignal, "github: empty response for project %s", p.spec.Ref())
	}
	return resp.Data.Search, nil
}

var projectItemFields = `title url body createdAt updatedAt state repository{nameWithOwner} author{login} ` +
	`comments(last:` + strconv.Itoa(commentWindow) + `){nodes{createdAt author{login __typename}}} ` +
	`assignees(first:20){nodes{login}} labels(first:30){nodes{name}} ` +
	`projectItems(first:20){nodes{project{number owner{... on Organization{login} ... on User{login}}} ` +
	`status: fieldValueByName(name:$field){... on ProjectV2ItemFieldSingleSelectValue{name}}}}`

var projectSearchQuery = `query($q:String!,$field:String!,$after:String){
  search(query:$q,type:ISSUE,first:` + strconv.Itoa(searchPageSize) + `,after:$after){
    issueCount
    pageInfo{hasNextPage endCursor}
    nodes{
      __typename
      ... on Issue{` + projectItemFields + `}
      ... on PullRequest{isDraft ` + projectItemFields + `}
    }
  }
}`

func (p *projectSignal) viewerLogin(ctx context.Context) (string, error) {
	raw, err := p.backend.GraphQL(ctx, `query{viewer{login}}`, nil)
	if err != nil {
		return "", err
	}
	var resp graphQLResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", errs.Wrap(errs.KindSignal, err, "github: decoding viewer response")
	}
	if err := resp.err(); err != nil {
		return "", err
	}
	if resp.Data.Viewer == nil || resp.Data.Viewer.Login == "" {
		return "", errs.New(errs.KindAuth, "github: cannot resolve @me — no authenticated user").
			WithHint("run `gh auth login` or `mino login github`")
	}
	return resp.Data.Viewer.Login, nil
}
