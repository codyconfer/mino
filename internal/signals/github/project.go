package github

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/signals"
)

const projectScopeHint = "the read:project scope is required; run `gh auth refresh -s read:project` or re-run `munin login github`"

const (
	searchPageSize  = 50
	searchMaxPages  = 20
	statusFieldName = "Status"
)

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
}

func NewProject(spec ProjectSpec, backend Backend, max int, cache RosterCache) signals.Signal {
	if spec.Field == "" {
		spec.Field = statusFieldName
	}
	if max <= 0 {
		max = defaultPerPage
	}
	return &projectSignal{spec: spec, backend: backend, max: max, cache: cache}
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
	lastCommentTeam bool
}

func (p *projectSignal) Fetch(ctx context.Context) ([]signals.Section, error) {
	pf, err := parseProjectFilter(p.spec.Filter)
	if err != nil {
		return nil, err
	}
	viewer := ""
	if pf.needsViewer {
		viewer, err = p.viewerLogin(ctx)
		if err != nil {
			return nil, err
		}
	}
	var roster *Roster
	if p.spec.Team != "" {
		roster, err = ResolveTeam(ctx, p.backend, p.cache, p.spec.Team)
		if err != nil {
			return nil, err
		}
	}
	search := pf.searchQuery(p.spec.Ref(), viewer)
	local := pf.local()

	items := make([]signals.Item, 0, p.max)
	cursor := ""
	for range searchMaxPages {
		res, err := p.page(ctx, search, cursor)
		if err != nil {
			return nil, err
		}
		for _, node := range res.Nodes {
			it, ok := node.toProjectItem(p.spec, roster)
			if !ok || !local.keeps(it, viewer) {
				continue
			}
			items = append(items, it.item)
			if len(items) >= p.max {
				return []signals.Section{p.section(items)}, nil
			}
		}
		if !res.PageInfo.HasNextPage || res.PageInfo.EndCursor == "" {
			break
		}
		cursor = res.PageInfo.EndCursor
	}
	return []signals.Section{p.section(items)}, nil
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
			Author *struct {
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
	it.lastCommentBy = lastResponder(n)
	it.lastCommentTeam = roster.Has(it.lastCommentBy)

	var ts time.Time
	if n.UpdatedAt != "" {
		ts, _ = time.Parse(time.RFC3339, n.UpdatedAt)
	}
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
	Errors []struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"errors"`
}

func (r graphQLResponse) err() error { return r.errHint(projectScopeHint) }

func (r graphQLResponse) errHint(scopeHint string) error {
	if len(r.Errors) == 0 {
		return nil
	}
	msgs := make([]string, 0, len(r.Errors))
	scopes := false
	for _, e := range r.Errors {
		msgs = append(msgs, e.Message)
		if e.Type == "INSUFFICIENT_SCOPES" {
			scopes = true
		}
	}
	joined := strings.Join(msgs, "; ")
	if scopes {
		return errs.Newf(errs.KindAuth, "github: graphql: %s", joined).WithHint("%s", scopeHint)
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

var projectItemFields = `title url body updatedAt state repository{nameWithOwner} author{login} ` +
	`comments(last:` + strconv.Itoa(commentWindow) + `){nodes{author{login __typename}}} ` +
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
			WithHint("run `gh auth login` or `munin login github`")
	}
	return resp.Data.Viewer.Login, nil
}
