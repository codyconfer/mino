package github

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/signals"
)

type RepositoryRef struct {
	Owner string
	Repo  string
}

func (r RepositoryRef) String() string { return r.Owner + "/" + r.Repo }

func ParseRepositoryRef(raw string) (RepositoryRef, error) {
	parts := strings.Split(strings.Trim(strings.TrimSpace(raw), "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return RepositoryRef{}, errs.Newf(errs.KindConfig, "github: invalid repository %q", raw).
			WithHint("use owner/repository, e.g. codyconfer/mino")
	}
	return RepositoryRef{Owner: parts[0], Repo: parts[1]}, nil
}

type actionsSignal struct {
	repo    RepositoryRef
	backend ActionsBackend
	title   string
}

func NewActions(repo RepositoryRef, backend ActionsBackend, opts ...Option) signals.Signal {
	o := applyOptions(opts)
	title := o.title
	if title == "" {
		title = repo.String() + " · latest CI"
	}
	return &actionsSignal{repo: repo, backend: backend, title: title}
}

func (s *actionsSignal) Name() string { return "github" }

func (s *actionsSignal) Fetch(ctx context.Context) ([]signals.Section, error) {
	raw, err := s.backend.WorkflowRuns(ctx, s.repo.Owner, s.repo.Repo, 1)
	if err != nil {
		wrapped := errs.Wrapf(errs.KindOf(err), err, "github: latest workflow run for %s", s.repo)
		if hint := errs.Hint(err); hint != "" {
			wrapped = wrapped.WithHint("%s", hint)
		}
		return nil, wrapped
	}
	section, err := mapWorkflowRuns(raw, s.repo, s.title)
	if err != nil {
		return nil, err
	}
	return []signals.Section{section}, nil
}

type workflowRunsResponse struct {
	TotalCount int `json:"total_count"`
	Runs       []struct {
		ID           int64  `json:"id"`
		Name         string `json:"name"`
		DisplayTitle string `json:"display_title"`
		Status       string `json:"status"`
		Conclusion   string `json:"conclusion"`
		HTMLURL      string `json:"html_url"`
		HeadBranch   string `json:"head_branch"`
		HeadSHA      string `json:"head_sha"`
		Event        string `json:"event"`
		RunNumber    int    `json:"run_number"`
		CreatedAt    string `json:"created_at"`
		UpdatedAt    string `json:"updated_at"`
	} `json:"workflow_runs"`
}

func mapWorkflowRuns(raw []byte, repo RepositoryRef, title string) (signals.Section, error) {
	var response workflowRunsResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return signals.Section{}, errs.Wrap(errs.KindSignal, err, "github: decoding workflow runs response")
	}
	section := signals.Section{Signal: "github", Title: title, Meta: map[string]string{
		"shown": strconv.Itoa(len(response.Runs)), "total": strconv.Itoa(response.TotalCount),
	}}
	for _, run := range response.Runs {
		state := workflowState(run.Status, run.Conclusion)
		name := run.Name
		if name == "" {
			name = "CI"
		}
		var timestamp time.Time
		if run.UpdatedAt != "" {
			timestamp, _ = time.Parse(time.RFC3339, run.UpdatedAt)
		}
		section.Items = append(section.Items, signals.Item{
			Kind:      "workflow",
			Title:     name + " #" + strconv.Itoa(run.RunNumber),
			Subtitle:  repo.String() + " · " + state,
			Body:      run.DisplayTitle,
			URL:       run.HTMLURL,
			Timestamp: timestamp,
			Meta: map[string]string{
				"repo": repo.String(), "owner": repo.Owner, "repository": repo.Repo,
				"run_id": strconv.FormatInt(run.ID, 10), "status": run.Status,
				"conclusion": run.Conclusion, "state": state, "branch": run.HeadBranch,
				"sha": run.HeadSHA, "event": run.Event,
			},
		})
	}
	return section, nil
}
