package gitlab

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/signals"
)

type userWire struct {
	Username string `json:"username"`
	Name     string `json:"name"`
	Bot      bool   `json:"bot"`
}

type milestoneWire struct {
	Title string `json:"title"`
}

type referencesWire struct {
	Full string `json:"full"`
}

type mergeRequestWire struct {
	ID                  int64          `json:"id"`
	IID                 int64          `json:"iid"`
	ProjectID           int64          `json:"project_id"`
	Title               string         `json:"title"`
	Description         string         `json:"description"`
	State               string         `json:"state"`
	Draft               bool           `json:"draft"`
	WebURL              string         `json:"web_url"`
	UpdatedAt           string         `json:"updated_at"`
	CreatedAt           string         `json:"created_at"`
	MergedAt            string         `json:"merged_at"`
	SourceBranch        string         `json:"source_branch"`
	TargetBranch        string         `json:"target_branch"`
	Labels              []string       `json:"labels"`
	MergeStatus         string         `json:"merge_status"`
	DetailedMergeStatus string         `json:"detailed_merge_status"`
	ChangesCount        string         `json:"changes_count"`
	Author              userWire       `json:"author"`
	Assignees           []userWire     `json:"assignees"`
	Reviewers           []userWire     `json:"reviewers"`
	Milestone           *milestoneWire `json:"milestone"`
	References          referencesWire `json:"references"`
}

func (w mergeRequestWire) project() string {
	if p := projectFromReference(w.References.Full, "!"); p != "" {
		return p
	}
	if p := projectPathFromWebURL(w.WebURL); p != "" {
		return p
	}
	if w.ProjectID > 0 {
		return strconv.FormatInt(w.ProjectID, 10)
	}
	return ""
}

func (w mergeRequestWire) item() signals.Item {
	project := w.project()
	meta := map[string]string{
		"state":    w.State,
		"iid":      strconv.FormatInt(w.IID, 10),
		"id":       strconv.FormatInt(w.ID, 10),
		"updated":  w.UpdatedAt,
		"web_url":  w.WebURL,
		"project":  project,
		"branches": w.SourceBranch + " -> " + w.TargetBranch,
	}
	putIf(meta, "author", w.Author.Username)
	putIf(meta, "source_branch", w.SourceBranch)
	putIf(meta, "target_branch", w.TargetBranch)
	putIf(meta, "labels", joinNames(w.Labels))
	putIf(meta, "assignees", atNames(usernames(w.Assignees)))
	putIf(meta, "reviewers", atNames(usernames(w.Reviewers)))
	putIf(meta, "merge_status", w.MergeStatus)
	putIf(meta, "detailed_merge_status", w.DetailedMergeStatus)
	putIf(meta, "changes_count", w.ChangesCount)
	if w.Milestone != nil {
		putIf(meta, "milestone", w.Milestone.Title)
	}
	if w.Draft {
		meta["draft"] = "true"
	}

	return signals.Item{
		Kind:      "mr",
		Title:     w.Title,
		Subtitle:  project,
		Body:      w.Description,
		URL:       w.WebURL,
		Timestamp: parseTime(w.UpdatedAt),
		Meta:      meta,
	}
}

func usernames(list []userWire) []string {
	out := make([]string, 0, len(list))
	for _, u := range list {
		if u.Username != "" {
			out = append(out, u.Username)
		}
	}
	return out
}

func projectFromReference(full, sep string) string {
	if full == "" {
		return ""
	}
	for i := range full {
		if string(full[i]) == sep {
			return full[:i]
		}
	}
	return ""
}

func fetchMergeRequests(ctx context.Context, b Backend, sel Selector, perPage, max int) ([]signals.Item, pageMeta, error) {
	var items []signals.Item
	decode := func(body []byte, room int) (int, error) {
		var rows []mergeRequestWire
		if err := json.Unmarshal(body, &rows); err != nil {
			return 0, errs.Wrap(errs.KindSignal, err, "gitlab: decoding merge requests")
		}
		n := min(len(rows), room)
		for _, w := range rows[:n] {
			items = append(items, w.item())
		}
		return n, nil
	}
	m, err := collect(ctx, b, sel.Path(), sel.Query(), perPage, max, decode)
	return items, m, err
}
