package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/signals"
)

type pipelineDetailWire struct {
	pipelineWire
	FinishedAt     string   `json:"finished_at"`
	Duration       *int     `json:"duration"`
	QueuedDuration *float64 `json:"queued_duration"`
	User           userWire `json:"user"`
}

type jobWire struct {
	ID         int64    `json:"id"`
	Name       string   `json:"name"`
	Stage      string   `json:"stage"`
	Status     string   `json:"status"`
	WebURL     string   `json:"web_url"`
	Duration   *float64 `json:"duration"`
	AllowFail  bool     `json:"allow_failure"`
	StartedAt  string   `json:"started_at"`
	FinishedAt string   `json:"finished_at"`
}

func jobDuration(w jobWire) string {
	if w.Duration == nil || *w.Duration <= 0 {
		return ""
	}
	return time.Duration(*w.Duration * float64(time.Second)).Round(time.Second).String()
}

func (s *Signal) pipelineDetail(ctx context.Context, ref Ref, it signals.Item) (signals.ItemDetail, error) {
	raw, err := s.get(ctx, ref.path(""), nil)
	if err != nil {
		return signals.ItemDetail{}, err
	}
	var w pipelineDetailWire
	if err := json.Unmarshal(raw, &w); err != nil {
		return signals.ItemDetail{}, errs.Wrap(errs.KindSignal, err, "gitlab: decoding pipeline")
	}
	jobs := s.fetchJobs(ctx, ref.Project, ref.ID)

	title := w.title()
	if strings.TrimSpace(w.Name) == "" && it.Title != "" {
		title = it.Title
	}
	d := signals.ItemDetail{
		Kind:  "pipeline",
		Title: title,
		URL:   w.WebURL,
		Meta:  map[string]string{"state": w.Status},
		Chips: []signals.Chip{chip(w.Status, pipelineSeverity(w.Status))},
	}
	if w.Source != "" {
		d.Chips = append(d.Chips, chip(w.Source, glyph.SeverityNeutral))
	}

	rows := [][2]string{{"project", ref.Project}}
	rows = row(rows, "ref", w.Ref)
	rows = row(rows, "commit", shortSHA(w.SHA))
	rows = row(rows, "triggered by", atLogin(w.User.Username))
	rows = row(rows, "duration", pipelineDuration(w))
	rows = row(rows, "created", relTime(w.CreatedAt))
	rows = row(rows, "updated", relTime(w.UpdatedAt))
	rows = row(rows, "finished", relTime(w.FinishedAt))
	d.Rows = rows

	if sec, ok := pipelineSection(&w.pipelineWire, jobs); ok {
		d.Sections = append(d.Sections, sec)
		if sec.Meta["in_progress"] == "true" {
			d.Meta["in_progress"] = "true"
		}
	}
	return d, nil
}

func pipelineSection(p *pipelineWire, jobs []jobWire) (signals.DetailSection, bool) {
	if p == nil {
		return signals.DetailSection{}, false
	}
	number := p.IID
	if number == 0 {
		number = p.ID
	}
	sec := signals.DetailSection{
		Title: fmt.Sprintf("pipeline · #%d", number),
		Meta: map[string]string{
			"pipeline_id": strconv.FormatInt(p.ID, 10),
			"url":         p.WebURL,
		},
	}
	running := jobInProgress(p.Status)
	for _, j := range jobs {
		if jobInProgress(j.Status) {
			running = true
		}
		sec.Rows = append(sec.Rows, [2]string{jobLabel(j), jobValue(j)})
	}
	if running {
		sec.Meta["in_progress"] = "true"
	}
	if len(sec.Rows) == 0 && p.Status == "" {
		return signals.DetailSection{}, false
	}
	return sec, true
}

func jobLabel(j jobWire) string {
	if j.Stage == "" {
		return j.Name
	}
	return "[" + j.Stage + "] " + j.Name
}

func jobValue(j jobWire) string {
	status := j.Status
	if j.AllowFail && strings.EqualFold(status, "failed") {
		status += " (allowed)"
	}
	if d := jobDuration(j); d != "" {
		return status + " · " + d
	}
	return status
}

func pipelineDuration(w pipelineDetailWire) string {
	total := 0.0
	if w.Duration != nil {
		total += float64(*w.Duration)
	}
	if w.QueuedDuration != nil {
		total += *w.QueuedDuration
	}
	if total <= 0 {
		return ""
	}
	return time.Duration(total * float64(time.Second)).Round(time.Second).String()
}

func shortSHA(sha string) string {
	if sha == "" {
		return ""
	}
	return sha[:min(len(sha), 12)]
}
