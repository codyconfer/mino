package gitea

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/codyconfer/viewkit/glyph"
	"github.com/codyconfer/viewkit/timefmt"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/signals"
)

const (
	detailCacheNS  = "gitea:detail"
	detailComments = 20
	detailReviews  = 20
)

type Cache interface {
	Get(ctx context.Context, namespace, key string) (string, bool)
	Put(ctx context.Context, namespace, key, value string, expiry time.Time)
}

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
	viewer string
}

type Option func(*signalOpts)

func WithDetailCache(c Cache, pol CachePolicy) Option {
	return func(o *signalOpts) { o.detail, o.policy = c, pol }
}

func WithTitle(title string) Option {
	return func(o *signalOpts) { o.title = title }
}

func WithViewer(login string) Option {
	return func(o *signalOpts) { o.viewer = strings.TrimSpace(login) }
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

func (r Ref) path(suffix string) string {
	return fmt.Sprintf("/repos/%s/%s/issues/%d%s", escapePath(r.Owner), escapePath(r.Repo), r.Number, suffix)
}

func (r Ref) pullPath(suffix string) string {
	return fmt.Sprintf("/repos/%s/%s/pulls/%d%s", escapePath(r.Owner), escapePath(r.Repo), r.Number, suffix)
}

func ParseRef(rawURL string) (Ref, error) {
	raw := strings.TrimSpace(rawURL)
	if raw == "" {
		return Ref{}, errs.New(errs.KindUsage, "gitea: empty issue or pull request URL")
	}
	badRef := func() error {
		return errs.Newf(errs.KindUsage, "gitea: cannot parse issue or pull request URL %q", rawURL).
			WithHint("use a URL like https://git.example.com/owner/repo/pulls/123")
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
	ref, err := ParseRef(it.URL)
	if err != nil {
		return signals.ItemDetail{}, err
	}
	node, err := loadDetail(ctx, s.backend, s.detail, s.policy, ref)
	if err != nil {
		return signals.ItemDetail{}, err
	}
	return node.toDetail(ref), nil
}

func loadDetail(ctx context.Context, b Backend, c Cache, pol CachePolicy, ref Ref) (*detailNode, error) {
	key := ref.String()
	if c != nil && pol.reads() {
		if raw, ok := c.Get(ctx, detailCacheNS, key); ok {
			var node detailNode
			if err := json.Unmarshal([]byte(raw), &node); err == nil {
				log.Debugf("gitea: detail cache hit %s", key)
				return &node, nil
			}
			log.Debugf("gitea: discarding unreadable detail cache entry %s", key)
		}
	}
	node, err := requestDetail(ctx, b, ref)
	if err != nil {
		return nil, err
	}
	if c != nil && pol.writes() {
		if raw, err := json.Marshal(node); err != nil {
			log.Debugf("gitea: detail cache encode failed: %v", err)
		} else {
			c.Put(ctx, detailCacheNS, key, string(raw), time.Now().Add(pol.TTL))
		}
	}
	return node, nil
}

func requestDetail(ctx context.Context, b Backend, ref Ref) (*detailNode, error) {
	raw, err := b.Issue(ctx, ref)
	if err != nil {
		return nil, errs.Wrapf(errs.KindOf(err), err, "gitea: %s", ref)
	}
	node := &detailNode{}
	if err := json.Unmarshal(raw, &node.Issue); err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "gitea: decoding issue response")
	}
	if node.Issue.Number == 0 {
		return nil, errs.Newf(errs.KindUsage, "gitea: no issue or pull request %s", ref)
	}
	node.PR = node.Issue.PullRequest != nil

	var wg sync.WaitGroup
	var commentsErr, pullErr, reviewsErr error

	if node.Issue.Comments > 0 {
		page := (node.Issue.Comments + detailComments - 1) / detailComments
		wg.Add(1)
		go func() {
			defer wg.Done()
			body, err := b.IssueComments(ctx, ref, page, detailComments)
			if err != nil {
				commentsErr = err
				return
			}
			commentsErr = json.Unmarshal(body, &node.Comments)
		}()
	}
	if node.PR {
		wg.Add(2)
		go func() {
			defer wg.Done()
			body, err := b.PullRequest(ctx, ref)
			if err != nil {
				pullErr = err
				return
			}
			pullErr = json.Unmarshal(body, &node.Pull)
		}()
		go func() {
			defer wg.Done()
			body, err := b.PullReviews(ctx, ref, detailReviews)
			if err != nil {
				reviewsErr = err
				return
			}
			reviewsErr = json.Unmarshal(body, &node.Reviews)
		}()
	}
	wg.Wait()

	for _, err := range []error{commentsErr, pullErr, reviewsErr} {
		if err != nil {
			log.Debugf("gitea: detail %s: %v", ref, err)
		}
	}
	return node, nil
}

type detailIssue struct {
	Number    int64  `json:"number"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	HTMLURL   string `json:"html_url"`
	State     string `json:"state"`
	Comments  int    `json:"comments"`
	CreatedAt string `json:"created_at"`
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
	Milestone *struct {
		Title string `json:"title"`
	} `json:"milestone"`
	PullRequest *struct {
		Merged bool `json:"merged"`
		Draft  bool `json:"draft"`
	} `json:"pull_request"`
}

type detailPull struct {
	Merged             bool   `json:"merged"`
	MergedAt           string `json:"merged_at"`
	Mergeable          *bool  `json:"mergeable"`
	Draft              bool   `json:"draft"`
	Additions          int    `json:"additions"`
	Deletions          int    `json:"deletions"`
	ChangedFiles       int    `json:"changed_files"`
	RequestedReviewers []struct {
		Login string `json:"login"`
	} `json:"requested_reviewers"`
}

type detailReview struct {
	State       string `json:"state"`
	SubmittedAt string `json:"submitted_at"`
	Stale       bool   `json:"stale"`
	User        *struct {
		Login string `json:"login"`
	} `json:"user"`
}

type detailComment struct {
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
	User      *struct {
		Login string `json:"login"`
	} `json:"user"`
}

type detailNode struct {
	Issue    detailIssue     `json:"issue"`
	Pull     detailPull      `json:"pull"`
	Reviews  []detailReview  `json:"reviews"`
	Comments []detailComment `json:"comments"`
	PR       bool            `json:"pr"`
}

func (n *detailNode) toDetail(ref Ref) signals.ItemDetail {
	kind := ref.Kind()
	if n.PR {
		kind = "pr"
	}
	d := signals.ItemDetail{
		Kind:  kind,
		Title: n.Issue.Title,
		URL:   n.Issue.HTMLURL,
		Body:  n.Issue.Body,
		Chips: n.chips(),
		Rows:  n.rows(ref),
	}
	if s, ok := n.reviewsSection(); ok {
		d.Sections = append(d.Sections, s)
	}
	if s, ok := n.commentsSection(); ok {
		d.Sections = append(d.Sections, s)
	}
	return d
}

func (n *detailNode) state() string {
	if n.PR && n.Pull.Merged {
		return "merged"
	}
	return strings.ToLower(n.Issue.State)
}

func (n *detailNode) chips() []signals.Chip {
	chips := []signals.Chip{{Label: n.state(), Sev: stateSeverity(n.state(), n.PR)}}
	if !n.PR {
		return chips
	}
	if n.Pull.Draft {
		chips = append(chips, signals.Chip{Label: "draft", Sev: glyph.SeverityNeutral})
	}
	if decision := reviewDecision(n.Reviews, len(n.Pull.RequestedReviewers)); decision != "" {
		chips = append(chips, signals.Chip{Label: decision, Sev: reviewSeverity(decision)})
	}
	if !n.Pull.Merged && n.Pull.Mergeable != nil && !*n.Pull.Mergeable {
		chips = append(chips, signals.Chip{Label: "conflicts", Sev: glyph.SeverityWarning})
	}
	return chips
}

func (n *detailNode) rows(ref Ref) [][2]string {
	rows := [][2]string{{"repo", ref.Slug() + " #" + strconv.Itoa(ref.Number)}}
	if n.Issue.User != nil && n.Issue.User.Login != "" {
		rows = append(rows, [2]string{"author", "@" + n.Issue.User.Login})
	}
	if labels := detailLabels(n.Issue); len(labels) > 0 {
		rows = append(rows, [2]string{"labels", strings.Join(labels, " · ")})
	}
	if who := detailAssignees(n.Issue); len(who) > 0 {
		rows = append(rows, [2]string{"assignees", strings.Join(who, ", ")})
	}
	if who := requestedReviewers(n.Pull); len(who) > 0 {
		rows = append(rows, [2]string{"reviewers", strings.Join(who, ", ")})
	}
	if n.Issue.Milestone != nil && n.Issue.Milestone.Title != "" {
		rows = append(rows, [2]string{"milestone", n.Issue.Milestone.Title})
	}
	if n.PR && n.Pull.ChangedFiles > 0 {
		rows = append(rows, [2]string{"diff", diffSummary(n.Pull.Additions, n.Pull.Deletions, n.Pull.ChangedFiles)})
	}
	if t := parseTime(n.Issue.CreatedAt); !t.IsZero() {
		rows = append(rows, [2]string{"created", timefmt.Rel(t)})
	}
	if t := parseTime(n.Issue.UpdatedAt); !t.IsZero() {
		rows = append(rows, [2]string{"updated", timefmt.Rel(t)})
	}
	if t := parseTime(n.Pull.MergedAt); !t.IsZero() {
		rows = append(rows, [2]string{"merged", timefmt.Rel(t)})
	}
	return rows
}

func detailLabels(it detailIssue) []string {
	out := make([]string, 0, len(it.Labels))
	for _, l := range it.Labels {
		out = append(out, l.Name)
	}
	return out
}

func detailAssignees(it detailIssue) []string {
	out := make([]string, 0, len(it.Assignees))
	for _, a := range it.Assignees {
		out = append(out, "@"+a.Login)
	}
	return out
}

func requestedReviewers(p detailPull) []string {
	out := make([]string, 0, len(p.RequestedReviewers))
	for _, r := range p.RequestedReviewers {
		if r.Login != "" {
			out = append(out, "@"+r.Login)
		}
	}
	return out
}

func reviewDecision(reviews []detailReview, requested int) string {
	latest := map[string]detailReview{}
	for _, r := range reviews {
		if r.User == nil || r.User.Login == "" {
			continue
		}
		if isCommentReview(r.State) {
			continue
		}
		prev, ok := latest[r.User.Login]
		if !ok || parseTime(r.SubmittedAt).After(parseTime(prev.SubmittedAt)) {
			latest[r.User.Login] = r
		}
	}
	approved := false
	for _, r := range latest {
		switch normalizeReviewState(r.State) {
		case "changes requested":
			return "changes requested"
		case "approved":
			approved = true
		}
	}
	if approved {
		return "approved"
	}
	if requested > 0 {
		return "review required"
	}
	return ""
}

func isCommentReview(state string) bool {
	switch normalizeReviewState(state) {
	case "comment", "pending":
		return true
	}
	return false
}

func normalizeReviewState(state string) string {
	s := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(state), "_", " "))
	switch s {
	case "request changes", "changes requested", "request_changes":
		return "changes requested"
	}
	return s
}

func reviewSeverity(decision string) glyph.Severity {
	switch decision {
	case "approved":
		return glyph.SeverityPositive
	case "changes requested":
		return glyph.SeverityWarning
	default:
		return glyph.SeverityNeutral
	}
}

func stateSeverity(state string, isPR bool) glyph.Severity {
	switch strings.ToLower(state) {
	case "merged":
		return glyph.SeverityPositive
	case "closed":
		if isPR {
			return glyph.SeverityNegative
		}
		return glyph.SeverityPositive
	default:
		return glyph.SeverityNeutral
	}
}

func (n *detailNode) reviewsSection() (signals.DetailSection, bool) {
	if len(n.Reviews) == 0 {
		return signals.DetailSection{}, false
	}
	rows := make([][2]string, 0, len(n.Reviews))
	for _, r := range n.Reviews {
		if r.User == nil || isCommentReview(r.State) {
			continue
		}
		state := normalizeReviewState(r.State)
		if t := parseTime(r.SubmittedAt); !t.IsZero() {
			state += " · " + timefmt.Rel(t)
		}
		if r.Stale {
			state += " · stale"
		}
		rows = append(rows, [2]string{"@" + r.User.Login, state})
	}
	if len(rows) == 0 {
		return signals.DetailSection{}, false
	}
	return signals.DetailSection{Title: "reviews", Rows: rows}, true
}

func (n *detailNode) commentsSection() (signals.DetailSection, bool) {
	if len(n.Comments) == 0 {
		return signals.DetailSection{}, false
	}
	var body strings.Builder
	for i, c := range n.Comments {
		if i > 0 {
			body.WriteString("\n\n---\n\n")
		}
		who := "unknown"
		if c.User != nil && c.User.Login != "" {
			who = "@" + c.User.Login
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
	if more := n.Issue.Comments - len(n.Comments); more > 0 {
		title += " (latest " + strconv.Itoa(len(n.Comments)) + " of " + strconv.Itoa(n.Issue.Comments) + ")"
	}
	return signals.DetailSection{Title: title, Body: body.String()}, true
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

func parseTime(raw string) time.Time {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return t
}
