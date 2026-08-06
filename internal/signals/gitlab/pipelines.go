package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/signals"
)

type pipelineWire struct {
	ID        int64  `json:"id"`
	IID       int64  `json:"iid"`
	ProjectID int64  `json:"project_id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Source    string `json:"source"`
	Ref       string `json:"ref"`
	SHA       string `json:"sha"`
	WebURL    string `json:"web_url"`
	UpdatedAt string `json:"updated_at"`
	CreatedAt string `json:"created_at"`
}

func (w pipelineWire) project(fallback string) string {
	if p := projectPathFromWebURL(w.WebURL); p != "" {
		return p
	}
	if fallback != "" {
		return fallback
	}
	if w.ProjectID > 0 {
		return strconv.FormatInt(w.ProjectID, 10)
	}
	return ""
}

func (w pipelineWire) title() string {
	name := strings.TrimSpace(w.Name)
	if name == "" {
		name = "pipeline"
	}
	number := w.IID
	if number == 0 {
		number = w.ID
	}
	return fmt.Sprintf("%s #%d", name, number)
}

func (w pipelineWire) item(project string) signals.Item {
	project = w.project(project)
	subtitle := project
	if w.Ref != "" {
		subtitle = project + " · " + w.Ref
	}
	meta := map[string]string{
		"status":  w.Status,
		"updated": w.UpdatedAt,
		"web_url": w.WebURL,
		"project": project,
		// pipeline_id is the global id: /projects/:id/pipelines/:id/jobs takes that, not the iid.
		"pipeline_id": strconv.FormatInt(w.ID, 10),
	}
	if w.IID > 0 {
		meta["iid"] = strconv.FormatInt(w.IID, 10)
	}
	putIf(meta, "ref", w.Ref)
	putIf(meta, "sha", w.SHA)
	putIf(meta, "source", w.Source)

	return signals.Item{
		Kind:      "pipeline",
		Title:     w.title(),
		Subtitle:  subtitle,
		URL:       w.WebURL,
		Timestamp: parseTime(w.UpdatedAt),
		Meta:      meta,
	}
}

func pipelineSeverity(status string) glyph.Severity {
	return signals.ClassifyState(status)
}

func jobInProgress(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running", "pending", "created", "manual", "preparing", "scheduled", "waiting_for_resource":
		return true
	}
	return false
}

func fetchPipelines(ctx context.Context, b Backend, sel Selector, perPage, max int) ([]signals.Item, pageMeta, error) {
	var items []signals.Item
	project := sel.Target.Path
	decode := func(body []byte, room int) (int, error) {
		var rows []pipelineWire
		if err := json.Unmarshal(body, &rows); err != nil {
			return 0, errs.Wrap(errs.KindSignal, err, "gitlab: decoding pipelines")
		}
		n := min(len(rows), room)
		for _, w := range rows[:n] {
			items = append(items, w.item(project))
		}
		return n, nil
	}
	m, err := collect(ctx, b, sel.Path(), sel.Query(), perPage, max, decode)
	return items, m, err
}
