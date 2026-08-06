package gitlab

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/signals"
)

type glyphSeverity = glyph.Severity

type approvalsWire struct {
	ApprovalsRequired int                       `json:"approvals_required"`
	ApprovalsLeft     int                       `json:"approvals_left"`
	ApprovedBy        []struct{ User userWire } `json:"approved_by"`
}

type diffWire struct {
	NewPath     string `json:"new_path"`
	OldPath     string `json:"old_path"`
	NewFile     bool   `json:"new_file"`
	RenamedFile bool   `json:"renamed_file"`
	DeletedFile bool   `json:"deleted_file"`
}

type detailNode struct {
	MR        *mergeRequestWire `json:"mr,omitempty"`
	Issue     *issueWire        `json:"issue,omitempty"`
	Approvals *approvalsWire    `json:"approvals,omitempty"`
	Pipeline  *pipelineWire     `json:"pipeline,omitempty"`
	Jobs      []jobWire         `json:"jobs,omitempty"`
	Diffs     []diffWire        `json:"diffs,omitempty"`
	Notes     []noteWire        `json:"notes,omitempty"`
	NoteTotal int               `json:"note_total,omitempty"`
}

const (
	notesPerPage = 20
	diffsPerPage = 20
)

func (s *Signal) requestDetail(ctx context.Context, ref Ref) (*detailNode, error) {
	node := &detailNode{}
	if ref.Kind == KindIssue {
		return node, s.requestIssueDetail(ctx, ref, node)
	}
	return node, s.requestMergeRequestDetail(ctx, ref, node)
}

func (s *Signal) requestIssueDetail(ctx context.Context, ref Ref, node *detailNode) error {
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		raw, err := s.get(gctx, ref.path(""), nil)
		if err != nil {
			return err
		}
		var w issueWire
		if err := json.Unmarshal(raw, &w); err != nil {
			return errs.Wrap(errs.KindSignal, err, "gitlab: decoding issue")
		}
		node.Issue = &w
		return nil
	})
	g.Go(func() error {
		notes, total, err := s.fetchNotes(gctx, ref)
		if err != nil {
			log.Debugf("gitlab: notes for %s: %v", ref, err)
			return nil
		}
		node.Notes, node.NoteTotal = notes, total
		return nil
	})
	return g.Wait()
}

func (s *Signal) requestMergeRequestDetail(ctx context.Context, ref Ref, node *detailNode) error {
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		raw, err := s.get(gctx, ref.path(""), nil)
		if err != nil {
			return err
		}
		var w mergeRequestWire
		if err := json.Unmarshal(raw, &w); err != nil {
			return errs.Wrap(errs.KindSignal, err, "gitlab: decoding merge request")
		}
		node.MR = &w
		return nil
	})
	// Approvals, diffs, the head pipeline and notes are best-effort: a token that can read
	// the MR but not one of these degrades to a missing section rather than no detail.
	g.Go(func() error {
		raw, err := s.get(gctx, ref.path("approvals"), nil)
		if err != nil {
			log.Debugf("gitlab: approvals for %s: %v", ref, err)
			return nil
		}
		var w approvalsWire
		if err := json.Unmarshal(raw, &w); err == nil {
			node.Approvals = &w
		}
		return nil
	})
	g.Go(func() error {
		raw, err := s.get(gctx, ref.path("pipelines"), url.Values{"per_page": {"1"}})
		if err != nil {
			log.Debugf("gitlab: head pipeline for %s: %v", ref, err)
			return nil
		}
		var rows []pipelineWire
		if err := json.Unmarshal(raw, &rows); err == nil && len(rows) > 0 {
			node.Pipeline = &rows[0]
			node.Jobs = s.fetchJobs(gctx, ref.Project, rows[0].ID)
		}
		return nil
	})
	g.Go(func() error {
		raw, err := s.get(gctx, ref.path("diffs"), url.Values{"per_page": {strconv.Itoa(diffsPerPage)}})
		if err != nil {
			log.Debugf("gitlab: diffs for %s: %v", ref, err)
			return nil
		}
		var rows []diffWire
		if err := json.Unmarshal(raw, &rows); err == nil {
			node.Diffs = rows
		}
		return nil
	})
	g.Go(func() error {
		notes, total, err := s.fetchNotes(gctx, ref)
		if err != nil {
			log.Debugf("gitlab: notes for %s: %v", ref, err)
			return nil
		}
		node.Notes, node.NoteTotal = notes, total
		return nil
	})
	return g.Wait()
}

func (s *Signal) fetchJobs(ctx context.Context, project string, pipelineID int64) []jobWire {
	path := "projects/" + encodeProject(project) + "/pipelines/" +
		strconv.FormatInt(pipelineID, 10) + "/jobs"
	raw, err := s.get(ctx, path, url.Values{"per_page": {"100"}})
	if err != nil {
		log.Debugf("gitlab: jobs for pipeline %d: %v", pipelineID, err)
		return nil
	}
	var rows []jobWire
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil
	}
	return rows
}

func (n *detailNode) mergeRequestDetail(ref Ref, it signals.Item) signals.ItemDetail {
	w := n.MR
	if w == nil {
		return signals.ItemDetail{Kind: "mr", Title: it.Title, URL: it.URL, Body: it.Body}
	}

	d := signals.ItemDetail{
		Kind:  "mr",
		Title: w.Title,
		URL:   w.WebURL,
		Body:  w.Description,
		Meta:  map[string]string{"state": w.State},
	}
	d.Chips = append(d.Chips, chip(w.State, mrStateSeverity(w.State)))
	if w.Draft {
		d.Chips = append(d.Chips, chip("draft", glyph.SeverityNeutral))
	}
	if label, ok := mergeBlocker(w.DetailedMergeStatus); ok {
		d.Chips = append(d.Chips, chip(label, glyph.SeverityWarning))
	}
	if n.Pipeline != nil {
		d.Chips = append(d.Chips, chip("pipeline "+n.Pipeline.Status, pipelineSeverity(n.Pipeline.Status)))
	}

	rows := [][2]string{{"project", ref.Project + " !" + strconv.FormatInt(ref.ID, 10)}}
	rows = row(rows, "author", atLogin(w.Author.Username))
	rows = row(rows, "branches", w.SourceBranch+" -> "+w.TargetBranch)
	rows = row(rows, "labels", strings.Join(w.Labels, " · "))
	rows = row(rows, "assignees", atNames(usernames(w.Assignees)))
	rows = row(rows, "reviewers", atNames(usernames(w.Reviewers)))
	if n.Approvals != nil {
		rows = row(rows, "approvals", approvalSummary(n.Approvals))
	}
	if w.Milestone != nil {
		rows = row(rows, "milestone", w.Milestone.Title)
	}
	rows = row(rows, "diff", changesSummary(w.ChangesCount))
	rows = row(rows, "created", relTime(w.CreatedAt))
	rows = row(rows, "updated", relTime(w.UpdatedAt))
	rows = row(rows, "merged", relTime(w.MergedAt))
	d.Rows = rows

	if sec, ok := pipelineSection(n.Pipeline, n.Jobs); ok {
		d.Sections = append(d.Sections, sec)
		if sec.Meta["in_progress"] == "true" {
			d.Meta["in_progress"] = "true"
		}
	}
	if sec, ok := approvalSection(n.Approvals); ok {
		d.Sections = append(d.Sections, sec)
	}
	if sec, ok := filesSection(n.Diffs, w.ChangesCount); ok {
		d.Sections = append(d.Sections, sec)
	}
	if sec, ok := notesSection(n.Notes, n.NoteTotal); ok {
		d.Sections = append(d.Sections, sec)
	}
	return d
}

func (n *detailNode) issueDetail(ref Ref, it signals.Item) signals.ItemDetail {
	w := n.Issue
	if w == nil {
		return signals.ItemDetail{Kind: "issue", Title: it.Title, URL: it.URL, Body: it.Body}
	}

	d := signals.ItemDetail{
		Kind:  "issue",
		Title: w.Title,
		URL:   w.WebURL,
		Body:  w.Description,
		Meta:  map[string]string{"state": w.State},
		Chips: []signals.Chip{chip(w.State, issueStateSeverity(w.State))},
	}
	if w.Confidential {
		d.Chips = append(d.Chips, chip("confidential", glyph.SeverityWarning))
	}

	rows := [][2]string{{"project", ref.Project + " #" + strconv.FormatInt(ref.ID, 10)}}
	rows = row(rows, "author", atLogin(w.Author.Username))
	rows = row(rows, "labels", strings.Join(w.Labels, " · "))
	rows = row(rows, "assignees", atNames(usernames(w.Assignees)))
	if w.Milestone != nil {
		rows = row(rows, "milestone", w.Milestone.Title)
	}
	rows = row(rows, "due date", w.DueDate)
	if w.Weight != nil {
		rows = row(rows, "weight", strconv.Itoa(*w.Weight))
	}
	rows = row(rows, "created", relTime(w.CreatedAt))
	rows = row(rows, "updated", relTime(w.UpdatedAt))
	rows = row(rows, "closed", relTime(w.ClosedAt))
	d.Rows = rows

	if sec, ok := notesSection(n.Notes, n.NoteTotal); ok {
		d.Sections = append(d.Sections, sec)
	}
	return d
}

func mrStateSeverity(state string) glyph.Severity {
	switch strings.ToLower(state) {
	case "merged":
		return glyph.SeverityPositive
	case "closed":
		return glyph.SeverityNegative
	case "locked":
		return glyph.SeverityWarning
	}
	return glyph.SeverityNeutral
}

func issueStateSeverity(state string) glyph.Severity {
	if strings.EqualFold(state, "closed") {
		return glyph.SeverityPositive
	}
	return glyph.SeverityNeutral
}

// mergeBlocker turns detailed_merge_status into a chip. It is GitLab's most useful
// "why can't I merge this" signal and has no GitHub equivalent.
func mergeBlocker(status string) (string, bool) {
	s := strings.ToLower(strings.TrimSpace(status))
	switch s {
	case "", "mergeable", "not_open", "checking", "unchecked":
		return "", false
	}
	return strings.ReplaceAll(s, "_", " "), true
}

func approvalSummary(a *approvalsWire) string {
	if a.ApprovalsRequired == 0 && len(a.ApprovedBy) == 0 {
		return ""
	}
	if a.ApprovalsRequired == 0 {
		return strconv.Itoa(len(a.ApprovedBy)) + " approved"
	}
	return strconv.Itoa(len(a.ApprovedBy)) + " of " + strconv.Itoa(a.ApprovalsRequired)
}

func approvalSection(a *approvalsWire) (signals.DetailSection, bool) {
	if a == nil || (len(a.ApprovedBy) == 0 && a.ApprovalsLeft == 0) {
		return signals.DetailSection{}, false
	}
	sec := signals.DetailSection{Title: "approvals"}
	for _, entry := range a.ApprovedBy {
		sec.Rows = append(sec.Rows, [2]string{atLogin(entry.User.Username), "approved"})
	}
	if a.ApprovalsLeft > 0 {
		sec.Lines = append(sec.Lines, strconv.Itoa(a.ApprovalsLeft)+" approval(s) still required")
	}
	return sec, true
}

// filesSection lists paths only. GitLab's /diffs returns raw patch text, so counting
// added and removed lines means reading megabytes to render twenty rows; the aggregate
// comes from changes_count instead.
func filesSection(diffs []diffWire, changesCount string) (signals.DetailSection, bool) {
	if len(diffs) == 0 {
		return signals.DetailSection{}, false
	}
	sec := signals.DetailSection{Title: "files"}
	for _, d := range diffs {
		path := d.NewPath
		if path == "" {
			path = d.OldPath
		}
		sec.Rows = append(sec.Rows, [2]string{path, diffVerb(d)})
	}
	if total, err := strconv.Atoi(strings.TrimSuffix(changesCount, "+")); err == nil && total > len(diffs) {
		sec.Lines = append(sec.Lines, "+"+strconv.Itoa(total-len(diffs))+" more")
	}
	return sec, true
}

func diffVerb(d diffWire) string {
	switch {
	case d.DeletedFile:
		return "deleted"
	case d.NewFile:
		return "added"
	case d.RenamedFile:
		return "renamed"
	}
	return "modified"
}

func changesSummary(count string) string {
	if count == "" {
		return ""
	}
	return count + " file(s) changed"
}

func atLogin(login string) string {
	if login == "" {
		return ""
	}
	return "@" + login
}
