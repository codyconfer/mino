package gitlab

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/signals"
)

type issueWire struct {
	ID           int64          `json:"id"`
	IID          int64          `json:"iid"`
	ProjectID    int64          `json:"project_id"`
	Title        string         `json:"title"`
	Description  string         `json:"description"`
	State        string         `json:"state"`
	WebURL       string         `json:"web_url"`
	UpdatedAt    string         `json:"updated_at"`
	CreatedAt    string         `json:"created_at"`
	ClosedAt     string         `json:"closed_at"`
	Labels       []string       `json:"labels"`
	Confidential bool           `json:"confidential"`
	DueDate      string         `json:"due_date"`
	Weight       *int           `json:"weight"`
	Author       userWire       `json:"author"`
	Assignees    []userWire     `json:"assignees"`
	Milestone    *milestoneWire `json:"milestone"`
	References   referencesWire `json:"references"`
}

func (w issueWire) project() string {
	if p := projectFromReference(w.References.Full, "#"); p != "" {
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

func (w issueWire) item() signals.Item {
	project := w.project()
	meta := map[string]string{
		"state":   w.State,
		"iid":     strconv.FormatInt(w.IID, 10),
		"id":      strconv.FormatInt(w.ID, 10),
		"updated": w.UpdatedAt,
		"web_url": w.WebURL,
		"project": project,
	}
	putIf(meta, "author", w.Author.Username)
	putIf(meta, "labels", joinNames(w.Labels))
	putIf(meta, "assignees", atNames(usernames(w.Assignees)))
	putIf(meta, "due_date", w.DueDate)
	if w.Milestone != nil {
		putIf(meta, "milestone", w.Milestone.Title)
	}
	if w.Confidential {
		meta["confidential"] = "true"
	}
	if w.Weight != nil {
		meta["weight"] = strconv.Itoa(*w.Weight)
	}

	return signals.Item{
		Kind:      "issue",
		Title:     w.Title,
		Subtitle:  project,
		Body:      w.Description,
		URL:       w.WebURL,
		Timestamp: parseTime(w.UpdatedAt),
		Meta:      meta,
	}
}

func fetchIssues(ctx context.Context, b Backend, sel Selector, perPage, max int) ([]signals.Item, pageMeta, error) {
	var items []signals.Item
	decode := func(body []byte, room int) (int, error) {
		var rows []issueWire
		if err := json.Unmarshal(body, &rows); err != nil {
			return 0, errs.Wrap(errs.KindSignal, err, "gitlab: decoding issues")
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
