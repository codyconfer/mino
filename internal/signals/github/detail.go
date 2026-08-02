package github

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/codyconfer/viewkit/glyph"
	"github.com/codyconfer/viewkit/timefmt"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/signals"
)

const detailScopeHint = "the repo scope is required; run `gh auth refresh -s repo` or re-run `mino login github`"

const (
	detailCacheNS   = "github:detail"
	detailComments  = 20
	detailFiles     = 20
	detailChecks    = 20
	detailSteps     = 100
	detailReviewers = 10
)

type CachePolicy struct {
	Read  bool
	Write bool
	TTL   time.Duration
}

func (p CachePolicy) reads() bool  { return p.Read && p.TTL > 0 }
func (p CachePolicy) writes() bool { return p.Write && p.TTL > 0 }

type signalOpts struct {
	detail Cache
	policy CachePolicy
	title  string
}

type Option func(*signalOpts)

func WithDetailCache(c Cache, pol CachePolicy) Option {
	return func(o *signalOpts) { o.detail, o.policy = c, pol }
}

// WithTitle names the section of a signal built from a single configured query,
// the way ProjectSpec.Title does for a project board. Signals running several
// queries keep their own per-query titles, since one name cannot cover them all.
func WithTitle(title string) Option {
	return func(o *signalOpts) { o.title = title }
}

func applyOptions(opts []Option) signalOpts {
	var o signalOpts
	for _, fn := range opts {
		fn(&o)
	}
	return o
}

type Ref struct {
	Owner  string
	Repo   string
	Number int
	IsPR   bool
}

func (r Ref) Slug() string { return r.Owner + "/" + r.Repo }

func (r Ref) String() string { return r.Slug() + "#" + strconv.Itoa(r.Number) }

func (r Ref) Kind() string {
	if r.IsPR {
		return "pr"
	}
	return "issue"
}

func ParseRef(rawURL string) (Ref, error) {
	raw := strings.TrimSpace(rawURL)
	if raw == "" {
		return Ref{}, errs.New(errs.KindUsage, "github: empty issue or pull request URL")
	}
	badRef := func() error {
		return errs.Newf(errs.KindUsage, "github: cannot parse issue or pull request URL %q", rawURL).
			WithHint("use a URL like https://github.com/owner/repo/pull/123")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return Ref{}, badRef()
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, seg := range parts {
		if i < 2 || i+1 >= len(parts) {
			continue
		}
		var isPR bool
		switch seg {
		case "pull", "pulls":
			isPR = true
		case "issues":
			isPR = false
		default:
			continue
		}
		n, err := strconv.Atoi(parts[i+1])
		if err != nil || n <= 0 {
			return Ref{}, badRef()
		}
		if parts[i-2] == "" || parts[i-1] == "" {
			return Ref{}, badRef()
		}
		return Ref{Owner: parts[i-2], Repo: parts[i-1], Number: n, IsPR: isPR}, nil
	}
	return Ref{}, badRef()
}

func (s *Signal) Detail(ctx context.Context, it signals.Item) (signals.ItemDetail, error) {
	if it.Kind == "workflow" || it.Meta["run_id"] != "" {
		return fetchWorkflowDetail(ctx, s.backend, it)
	}
	return fetchDetail(ctx, s.backend, s.detail, s.policy, it)
}

func (p *projectSignal) Detail(ctx context.Context, it signals.Item) (signals.ItemDetail, error) {
	return fetchDetail(ctx, p.backend, p.detail, p.policy, it)
}

func fetchDetail(ctx context.Context, b Backend, c Cache, pol CachePolicy, it signals.Item) (signals.ItemDetail, error) {
	ref, err := ParseRef(it.URL)
	if err != nil {
		return signals.ItemDetail{}, err
	}
	node, err := loadDetail(ctx, b, c, pol, ref)
	if err != nil {
		return signals.ItemDetail{}, err
	}
	return node.toDetail(ref), nil
}

func fetchWorkflowDetail(ctx context.Context, backend Backend, it signals.Item) (signals.ItemDetail, error) {
	actions, ok := backend.(ActionsBackend)
	if !ok {
		return signals.ItemDetail{}, errs.New(errs.KindInternal, "github: backend does not support Actions")
	}
	repo, err := ParseRepositoryRef(it.Meta["repo"])
	if err != nil {
		return signals.ItemDetail{}, err
	}
	runID, err := strconv.ParseInt(it.Meta["run_id"], 10, 64)
	if err != nil || runID <= 0 {
		return signals.ItemDetail{}, errs.Newf(errs.KindUsage, "github: invalid workflow run id %q", it.Meta["run_id"])
	}
	runRaw, err := actions.WorkflowRun(ctx, repo.Owner, repo.Repo, runID)
	if err != nil {
		return signals.ItemDetail{}, errs.Wrapf(errs.KindOf(err), err, "github: workflow run %d", runID)
	}
	var run struct {
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	}
	if err := json.Unmarshal(runRaw, &run); err != nil {
		return signals.ItemDetail{}, errs.Wrap(errs.KindSignal, err, "github: decoding workflow run response")
	}
	raw, err := actions.WorkflowJobs(ctx, repo.Owner, repo.Repo, runID)
	if err != nil {
		return signals.ItemDetail{}, errs.Wrapf(errs.KindOf(err), err, "github: workflow run %d jobs", runID)
	}
	var response struct {
		Jobs []workflowJob `json:"jobs"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return signals.ItemDetail{}, errs.Wrap(errs.KindSignal, err, "github: decoding workflow jobs response")
	}
	name := strings.TrimSpace(strings.Split(it.Title, " #")[0])
	if name == "" {
		name = "CI"
	}
	state := workflowState(run.Status, run.Conclusion)
	detail := signals.ItemDetail{
		Kind: "workflow", Title: it.Title, URL: it.URL, Body: it.Body,
		Chips: []signals.Chip{{Label: state, Sev: workflowSeverity(state)}},
		Rows:  [][2]string{{"repo", repo.String()}, {"branch", it.Meta["branch"]}, {"event", it.Meta["event"]}},
	}
	if sha := it.Meta["sha"]; sha != "" {
		detail.Rows = append(detail.Rows, [2]string{"commit", sha[:min(len(sha), 12)]})
	}
	if section, ok := (workflowRun{ID: runID, Name: name, URL: it.URL, Jobs: response.Jobs}).section(); ok {
		detail.Sections = append(detail.Sections, section)
	}
	return detail, nil
}

func loadDetail(ctx context.Context, b Backend, c Cache, pol CachePolicy, ref Ref) (*detailNode, error) {
	key := ref.String()
	if c != nil && pol.reads() {
		if raw, ok := c.Get(ctx, detailCacheNS, key); ok {
			var node detailNode
			if err := json.Unmarshal([]byte(raw), &node); err == nil {
				log.Debugf("github: detail cache hit %s", key)
				return &node, nil
			}
			log.Debugf("github: discarding unreadable detail cache entry %s", key)
		}
	}
	node, err := requestDetail(ctx, b, ref)
	if err != nil {
		return nil, err
	}
	if c != nil && pol.writes() {
		if raw, err := json.Marshal(node); err != nil {
			log.Debugf("github: detail cache encode failed: %v", err)
		} else {
			c.Put(ctx, detailCacheNS, key, string(raw), time.Now().Add(pol.TTL))
		}
	}
	return node, nil
}

func requestDetail(ctx context.Context, b Backend, ref Ref) (*detailNode, error) {
	vars := map[string]any{"owner": ref.Owner, "repo": ref.Repo, "n": ref.Number}
	raw, err := b.GraphQL(ctx, detailQuery, vars)
	if err != nil {
		return nil, err
	}
	var resp detailResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "github: decoding detail response")
	}
	if err := resp.err(); err != nil {
		return nil, err
	}
	if resp.Data.Repository == nil || resp.Data.Repository.Node == nil {
		return nil, errs.Newf(errs.KindUsage, "github: no issue or pull request %s", ref)
	}
	return resp.Data.Repository.Node, nil
}

var detailCommonFields = `title url body state createdAt updatedAt author{login} milestone{title} ` +
	`labels(first:30){nodes{name}} assignees(first:20){nodes{login}} ` +
	`comments(last:` + strconv.Itoa(detailComments) + `){totalCount nodes{createdAt body author{login __typename}}}`

var detailPRFields = ` isDraft merged mergedAt reviewDecision additions deletions changedFiles ` +
	`files(first:` + strconv.Itoa(detailFiles) + `){totalCount nodes{path additions deletions}} ` +
	`reviewRequests(first:` + strconv.Itoa(detailReviewers) + `){nodes{requestedReviewer{... on User{login} ... on Team{name}}}} ` +
	`latestReviews(first:20){nodes{state submittedAt author{login}}} ` +
	`commits(last:1){nodes{commit{statusCheckRollup{state contexts(first:` + strconv.Itoa(detailChecks) + `){nodes{` +
	`... on CheckRun{name conclusion status steps(first:` + strconv.Itoa(detailSteps) + `){nodes{name conclusion status}} ` +
	`checkSuite{workflowRun{url workflow{name}}}} ` +
	`... on StatusContext{context state}}}}}}}`

var detailQuery = `query($owner:String!,$repo:String!,$n:Int!){
  repository(owner:$owner,name:$repo){
    issueOrPullRequest(number:$n){
      __typename
      ... on Issue{` + detailCommonFields + `}
      ... on PullRequest{` + detailCommonFields + detailPRFields + `}
    }
  }
}`

type detailResponse struct {
	Data struct {
		Repository *struct {
			Node *detailNode `json:"issueOrPullRequest"`
		} `json:"repository"`
	} `json:"data"`
	graphQLErrors
}

func (r detailResponse) err() error { return r.errHint(detailScopeHint) }

type detailNode struct {
	TypeName  string `json:"__typename"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Body      string `json:"body"`
	State     string `json:"state"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
	Author    *struct {
		Login string `json:"login"`
	} `json:"author"`
	Milestone *struct {
		Title string `json:"title"`
	} `json:"milestone"`
	Labels struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"labels"`
	Assignees struct {
		Nodes []struct {
			Login string `json:"login"`
		} `json:"nodes"`
	} `json:"assignees"`
	Comments struct {
		TotalCount int `json:"totalCount"`
		Nodes      []struct {
			CreatedAt string `json:"createdAt"`
			Body      string `json:"body"`
			Author    *struct {
				Login    string `json:"login"`
				TypeName string `json:"__typename"`
			} `json:"author"`
		} `json:"nodes"`
	} `json:"comments"`
	IsDraft        bool   `json:"isDraft"`
	Merged         bool   `json:"merged"`
	MergedAt       string `json:"mergedAt"`
	ReviewDecision string `json:"reviewDecision"`
	Additions      int    `json:"additions"`
	Deletions      int    `json:"deletions"`
	ChangedFiles   int    `json:"changedFiles"`
	Files          struct {
		TotalCount int `json:"totalCount"`
		Nodes      []struct {
			Path      string `json:"path"`
			Additions int    `json:"additions"`
			Deletions int    `json:"deletions"`
		} `json:"nodes"`
	} `json:"files"`
	ReviewRequests struct {
		Nodes []struct {
			RequestedReviewer *struct {
				Login string `json:"login"`
				Name  string `json:"name"`
			} `json:"requestedReviewer"`
		} `json:"nodes"`
	} `json:"reviewRequests"`
	LatestReviews struct {
		Nodes []struct {
			State       string `json:"state"`
			SubmittedAt string `json:"submittedAt"`
			Author      *struct {
				Login string `json:"login"`
			} `json:"author"`
		} `json:"nodes"`
	} `json:"latestReviews"`
	Commits struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *checkRollup `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

type checkContext struct {
	Name       string `json:"name"`
	Conclusion string `json:"conclusion"`
	Status     string `json:"status"`
	Context    string `json:"context"`
	State      string `json:"state"`
	Steps      struct {
		Nodes []workflowStep `json:"nodes"`
	} `json:"steps"`
	CheckSuite *struct {
		WorkflowRun *struct {
			URL      string `json:"url"`
			Workflow *struct {
				Name string `json:"name"`
			} `json:"workflow"`
		} `json:"workflowRun"`
	} `json:"checkSuite"`
}

type checkRollup struct {
	State    string `json:"state"`
	Contexts struct {
		Nodes []checkContext `json:"nodes"`
	} `json:"contexts"`
}

type workflowRun struct {
	ID   int64         `json:"id"`
	Name string        `json:"name"`
	URL  string        `json:"url,omitempty"`
	Jobs []workflowJob `json:"jobs"`
}

type workflowJob struct {
	Name       string         `json:"name"`
	Status     string         `json:"status"`
	Conclusion string         `json:"conclusion"`
	Steps      []workflowStep `json:"steps"`
}

type workflowStep struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

func (n *detailNode) workflowRuns() []workflowRun {
	roll := n.rollup()
	if roll == nil {
		return nil
	}
	var runs []workflowRun
	byURL := make(map[string]int)
	for _, check := range roll.Contexts.Nodes {
		if check.CheckSuite == nil || check.CheckSuite.WorkflowRun == nil {
			continue
		}
		source := check.CheckSuite.WorkflowRun
		name := "GitHub Actions"
		if source.Workflow != nil && source.Workflow.Name != "" {
			name = source.Workflow.Name
		}
		idx, ok := byURL[source.URL]
		if !ok {
			idx = len(runs)
			byURL[source.URL] = idx
			runs = append(runs, workflowRun{ID: workflowRunID(source.URL), Name: name, URL: source.URL})
		}
		runs[idx].Jobs = append(runs[idx].Jobs, workflowJob{
			Name: check.Name, Status: check.Status, Conclusion: check.Conclusion, Steps: check.Steps.Nodes,
		})
	}
	return runs
}

func workflowRunID(raw string) int64 {
	u, err := url.Parse(raw)
	if err != nil {
		return 0
	}
	tail := strings.TrimSpace(strings.TrimRight(u.Path, "/"))
	tail = tail[strings.LastIndex(tail, "/")+1:]
	id, _ := strconv.ParseInt(tail, 10, 64)
	return id
}

func (n *detailNode) isPR() bool { return n.TypeName == "PullRequest" }

func (n *detailNode) toDetail(ref Ref) signals.ItemDetail {
	d := signals.ItemDetail{
		Kind:  ref.Kind(),
		Title: n.Title,
		URL:   n.URL,
		Body:  n.Body,
		Chips: n.chips(),
		Rows:  n.rows(ref),
	}
	if n.isPR() {
		if s, ok := n.checksSection(); ok {
			d.Sections = append(d.Sections, s)
		}
		for _, run := range n.workflowRuns() {
			if s, ok := run.section(); ok {
				d.Sections = append(d.Sections, s)
			}
		}
		if s, ok := n.reviewsSection(); ok {
			d.Sections = append(d.Sections, s)
		}
		if s, ok := n.filesSection(); ok {
			d.Sections = append(d.Sections, s)
		}
	}
	if s, ok := n.commentsSection(); ok {
		d.Sections = append(d.Sections, s)
	}
	return d
}

func (r workflowRun) section() (signals.DetailSection, bool) {
	if len(r.Jobs) == 0 {
		return signals.DetailSection{}, false
	}
	rows := make([][2]string, 0, len(r.Jobs)*2)
	inProgress := false
	for _, job := range r.Jobs {
		state := workflowState(job.Status, job.Conclusion)
		inProgress = inProgress || state == "in progress"
		rows = append(rows, [2]string{job.Name, state})
		for _, step := range job.Steps {
			state = workflowState(step.Status, step.Conclusion)
			inProgress = inProgress || state == "in progress"
			rows = append(rows, [2]string{"  ↳ " + step.Name, state})
		}
	}
	meta := map[string]string{
		"run_id": strconv.FormatInt(r.ID, 10),
		"url":    r.URL,
	}
	if inProgress {
		meta["in_progress"] = "true"
	}
	sec := signals.DetailSection{Title: "workflow · " + r.Name, Rows: rows, Meta: meta}
	return sec, true
}

func workflowState(status, conclusion string) string {
	state := conclusion
	if state == "" {
		state = status
	}
	return strings.ToLower(strings.ReplaceAll(state, "_", " "))
}

func (n *detailNode) chips() []signals.Chip {
	chips := []signals.Chip{{Label: strings.ToLower(n.State), Sev: stateSeverity(n.State, n.isPR())}}
	if n.IsDraft {
		chips = append(chips, signals.Chip{Label: "draft", Sev: glyph.SeverityNeutral})
	}
	if d := n.ReviewDecision; d != "" {
		chips = append(chips, signals.Chip{Label: reviewLabel(d), Sev: reviewSeverity(d)})
	}
	if roll := n.rollup(); roll != nil && roll.State != "" {
		chips = append(chips, signals.Chip{Label: "checks " + strings.ToLower(roll.State), Sev: checkSeverity(roll.State)})
	}
	return chips
}

func (n *detailNode) rows(ref Ref) [][2]string {
	rows := [][2]string{{"repo", ref.Slug() + " #" + strconv.Itoa(ref.Number)}}
	if n.Author != nil && n.Author.Login != "" {
		rows = append(rows, [2]string{"author", "@" + n.Author.Login})
	}
	if labels := n.labelNames(); len(labels) > 0 {
		rows = append(rows, [2]string{"labels", strings.Join(labels, " · ")})
	}
	if who := n.assigneeLogins(); len(who) > 0 {
		rows = append(rows, [2]string{"assignees", strings.Join(who, ", ")})
	}
	if who := n.reviewerNames(); len(who) > 0 {
		rows = append(rows, [2]string{"reviewers", strings.Join(who, ", ")})
	}
	if n.Milestone != nil && n.Milestone.Title != "" {
		rows = append(rows, [2]string{"milestone", n.Milestone.Title})
	}
	if n.isPR() && n.ChangedFiles > 0 {
		rows = append(rows, [2]string{"diff", diffSummary(n.Additions, n.Deletions, n.ChangedFiles)})
	}
	if t := parseTime(n.CreatedAt); !t.IsZero() {
		rows = append(rows, [2]string{"created", timefmt.Rel(t)})
	}
	if t := parseTime(n.UpdatedAt); !t.IsZero() {
		rows = append(rows, [2]string{"updated", timefmt.Rel(t)})
	}
	if t := parseTime(n.MergedAt); !t.IsZero() {
		rows = append(rows, [2]string{"merged", timefmt.Rel(t)})
	}
	return rows
}

func diffSummary(add, del, files int) string {
	return "+" + strconv.Itoa(add) + " −" + strconv.Itoa(del) + " across " +
		strconv.Itoa(files) + " " + plural(files, "file", "files")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func (n *detailNode) labelNames() []string {
	out := make([]string, 0, len(n.Labels.Nodes))
	for _, l := range n.Labels.Nodes {
		out = append(out, l.Name)
	}
	return out
}

func (n *detailNode) assigneeLogins() []string {
	out := make([]string, 0, len(n.Assignees.Nodes))
	for _, a := range n.Assignees.Nodes {
		out = append(out, "@"+a.Login)
	}
	return out
}

func (n *detailNode) reviewerNames() []string {
	out := make([]string, 0, len(n.ReviewRequests.Nodes))
	for _, r := range n.ReviewRequests.Nodes {
		switch {
		case r.RequestedReviewer == nil:
			continue
		case r.RequestedReviewer.Login != "":
			out = append(out, "@"+r.RequestedReviewer.Login)
		case r.RequestedReviewer.Name != "":
			out = append(out, r.RequestedReviewer.Name)
		}
	}
	return out
}

func (n *detailNode) rollup() *checkRollup {
	for _, c := range n.Commits.Nodes {
		if c.Commit.StatusCheckRollup != nil {
			return c.Commit.StatusCheckRollup
		}
	}
	return nil
}

func (n *detailNode) checksSection() (signals.DetailSection, bool) {
	roll := n.rollup()
	if roll == nil || len(roll.Contexts.Nodes) == 0 {
		return signals.DetailSection{}, false
	}
	rows := make([][2]string, 0, len(roll.Contexts.Nodes))
	seen := make(map[[2]string]bool, len(roll.Contexts.Nodes))
	for _, c := range roll.Contexts.Nodes {
		name, state := c.Name, c.Conclusion
		if name == "" {
			name = c.Context
		}
		if state == "" {
			state = c.State
		}
		if state == "" {
			state = c.Status
		}
		row := [2]string{name, strings.ToLower(state)}
		if seen[row] {
			continue
		}
		seen[row] = true
		rows = append(rows, row)
	}
	return signals.DetailSection{
		Title: "checks",
		Rows:  rows,
		Meta:  map[string]string{"state": strings.ToLower(roll.State)},
	}, true
}

func (n *detailNode) reviewsSection() (signals.DetailSection, bool) {
	if len(n.LatestReviews.Nodes) == 0 {
		return signals.DetailSection{}, false
	}
	rows := make([][2]string, 0, len(n.LatestReviews.Nodes))
	for _, r := range n.LatestReviews.Nodes {
		who := ""
		if r.Author != nil {
			who = "@" + r.Author.Login
		}
		state := reviewLabel(r.State)
		if t := parseTime(r.SubmittedAt); !t.IsZero() {
			state += " · " + timefmt.Rel(t)
		}
		rows = append(rows, [2]string{who, state})
	}
	return signals.DetailSection{Title: "reviews", Rows: rows}, true
}

func (n *detailNode) filesSection() (signals.DetailSection, bool) {
	if len(n.Files.Nodes) == 0 {
		return signals.DetailSection{}, false
	}
	rows := make([][2]string, 0, len(n.Files.Nodes))
	for _, f := range n.Files.Nodes {
		rows = append(rows, [2]string{f.Path, "+" + strconv.Itoa(f.Additions) + " −" + strconv.Itoa(f.Deletions)})
	}
	sec := signals.DetailSection{Title: "files", Rows: rows}
	if more := n.Files.TotalCount - len(n.Files.Nodes); more > 0 {
		sec.Lines = []string{"+" + strconv.Itoa(more) + " more"}
	}
	return sec, true
}

func (n *detailNode) commentsSection() (signals.DetailSection, bool) {
	if len(n.Comments.Nodes) == 0 {
		return signals.DetailSection{}, false
	}
	var body strings.Builder
	for i, c := range n.Comments.Nodes {
		if i > 0 {
			body.WriteString("\n\n---\n\n")
		}
		who := "unknown"
		if c.Author != nil && c.Author.Login != "" {
			who = "@" + c.Author.Login
			if isBotLogin(c.Author.Login, c.Author.TypeName) {
				who += " ·bot"
			}
		}
		body.WriteString("### ")
		body.WriteString(who)
		if t := parseTime(c.CreatedAt); !t.IsZero() {
			body.WriteString(" · ")
			body.WriteString(timefmt.Rel(t))
		}
		body.WriteString("\n\n")
		body.WriteString(strings.TrimSpace(c.Body))
	}
	title := "comments"
	if more := n.Comments.TotalCount - len(n.Comments.Nodes); more > 0 {
		title += " (latest " + strconv.Itoa(len(n.Comments.Nodes)) + " of " + strconv.Itoa(n.Comments.TotalCount) + ")"
	}
	return signals.DetailSection{Title: title, Body: body.String()}, true
}

func stateSeverity(state string, isPR bool) glyph.Severity {
	switch strings.ToUpper(state) {
	case "MERGED":
		return glyph.SeverityPositive
	case "CLOSED":
		if isPR {
			return glyph.SeverityNegative
		}
		return glyph.SeverityPositive
	default:
		return glyph.SeverityNeutral
	}
}

func reviewLabel(decision string) string {
	return strings.ToLower(strings.ReplaceAll(decision, "_", " "))
}

func reviewSeverity(decision string) glyph.Severity {
	switch strings.ToUpper(decision) {
	case "APPROVED":
		return glyph.SeverityPositive
	case "CHANGES_REQUESTED":
		return glyph.SeverityWarning
	default:
		return glyph.SeverityNeutral
	}
}

func checkSeverity(state string) glyph.Severity {
	switch strings.ToUpper(state) {
	case "SUCCESS":
		return glyph.SeverityPositive
	case "FAILURE", "ERROR":
		return glyph.SeverityNegative
	case "PENDING", "EXPECTED":
		return glyph.SeverityWarning
	default:
		return glyph.SeverityNeutral
	}
}

func workflowSeverity(state string) glyph.Severity {
	switch strings.ToUpper(strings.ReplaceAll(state, " ", "_")) {
	case "SUCCESS":
		return glyph.SeverityPositive
	case "FAILURE", "TIMED_OUT", "STARTUP_FAILURE":
		return glyph.SeverityNegative
	case "IN_PROGRESS", "QUEUED", "PENDING", "WAITING", "REQUESTED", "ACTION_REQUIRED":
		return glyph.SeverityWarning
	default:
		return glyph.SeverityNeutral
	}
}
