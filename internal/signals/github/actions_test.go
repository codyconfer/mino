package github

import (
	"context"
	"errors"
	"testing"

	"github.com/codyconfer/mino/internal/signals"
)

type fakeActionsBackend struct {
	runsOwner string
	runsRepo  string
	perPage   int
	jobsOwner string
	jobsRepo  string
	runID     int64
	runs      string
	run       string
	jobs      string
	err       error
}

func (f *fakeActionsBackend) WorkflowRun(_ context.Context, owner, repo string, runID int64) ([]byte, error) {
	f.runsOwner, f.runsRepo, f.runID = owner, repo, runID
	if f.err != nil {
		return nil, f.err
	}
	return []byte(f.run), nil
}

func (f *fakeActionsBackend) WorkflowRuns(_ context.Context, owner, repo string, perPage int) ([]byte, error) {
	f.runsOwner, f.runsRepo, f.perPage = owner, repo, perPage
	if f.err != nil {
		return nil, f.err
	}
	return []byte(f.runs), nil
}

func (f *fakeActionsBackend) WorkflowJobs(_ context.Context, owner, repo string, runID int64) ([]byte, error) {
	f.jobsOwner, f.jobsRepo, f.runID = owner, repo, runID
	if f.err != nil {
		return nil, f.err
	}
	return []byte(f.jobs), nil
}

func TestParseRepositoryRef(t *testing.T) {
	got, err := ParseRepositoryRef("/codyconfer/mino/")
	if err != nil {
		t.Fatal(err)
	}
	if got.Owner != "codyconfer" || got.Repo != "mino" || got.String() != "codyconfer/mino" {
		t.Fatalf("repository = %#v", got)
	}
	for _, raw := range []string{"", "mino", "a/b/c", "/a//b"} {
		if _, err := ParseRepositoryRef(raw); err == nil {
			t.Errorf("ParseRepositoryRef(%q) succeeded", raw)
		}
	}
}

func TestActionsFetchMapsLatestRun(t *testing.T) {
	backend := &fakeActionsBackend{runs: `{
  "total_count":2,
  "workflow_runs":[{
    "id":30706047121,"name":"test","display_title":"Tighten status rendering",
    "status":"in_progress","conclusion":null,
    "html_url":"https://github.com/codyconfer/mino/actions/runs/30706047121",
    "head_branch":"main","head_sha":"f24b8489b064effc993e8d2ba5c4b8ca4481b35e",
    "event":"push","run_number":42,"updated_at":"2026-07-31T12:00:00Z"
  }]}`}
	sig := NewActions(RepositoryRef{Owner: "codyconfer", Repo: "mino"}, backend, WithTitle("mino · latest CI"))
	sections, err := sig.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if backend.runsOwner != "codyconfer" || backend.runsRepo != "mino" || backend.perPage != 1 {
		t.Fatalf("request = %s/%s per_page=%d", backend.runsOwner, backend.runsRepo, backend.perPage)
	}
	if len(sections) != 1 || sections[0].Title != "mino · latest CI" || len(sections[0].Items) != 1 {
		t.Fatalf("sections = %#v", sections)
	}
	item := sections[0].Items[0]
	if item.Kind != "workflow" || item.Title != "test #42" || item.Body != "Tighten status rendering" {
		t.Fatalf("item = %#v", item)
	}
	if item.Meta["run_id"] != "30706047121" || item.Meta["state"] != "in progress" || item.Meta["branch"] != "main" {
		t.Fatalf("meta = %#v", item.Meta)
	}
}

func TestActionsFetchPreservesBackendError(t *testing.T) {
	backend := &fakeActionsBackend{err: errors.New("unavailable")}
	sig := NewActions(RepositoryRef{Owner: "codyconfer", Repo: "mino"}, backend)
	if _, err := sig.Fetch(context.Background()); err == nil {
		t.Fatal("expected fetch error")
	}
}

type workflowDetailBackend struct {
	*fakeActionsBackend
}

func (f *workflowDetailBackend) SearchIssues(context.Context, string, int) ([]byte, error) {
	return nil, errors.New("unexpected search")
}

func (f *workflowDetailBackend) GraphQL(context.Context, string, map[string]any) ([]byte, error) {
	return nil, errors.New("unexpected graphql")
}

func TestWorkflowDetailMapsJobsAndSteps(t *testing.T) {
	backend := &workflowDetailBackend{fakeActionsBackend: &fakeActionsBackend{
		run: `{"status":"in_progress","conclusion":null}`,
		jobs: `{"jobs":[{
  "id":91385242678,"name":"test","status":"in_progress","conclusion":null,
  "html_url":"https://github.com/codyconfer/mino/actions/runs/30706047121/job/91385242678",
  "steps":[
    {"number":1,"name":"Set up job","status":"completed","conclusion":"success"},
    {"number":2,"name":"Run tests","status":"in_progress","conclusion":null}
  ]
}]}`}}
	item := signals.Item{
		Kind: "workflow", Title: "test #42", Body: "Tighten status rendering",
		URL: "https://github.com/codyconfer/mino/actions/runs/30706047121",
		Meta: map[string]string{
			"repo": "codyconfer/mino", "run_id": "30706047121", "state": "queued",
			"branch": "main", "event": "push", "sha": "f24b8489b064effc993e8d2ba5c4b8ca4481b35e",
		},
	}
	detail, err := fetchWorkflowDetail(context.Background(), backend, item)
	if err != nil {
		t.Fatal(err)
	}
	if backend.jobsOwner != "codyconfer" || backend.jobsRepo != "mino" || backend.runID != 30706047121 {
		t.Fatalf("jobs request = %s/%s run=%d", backend.jobsOwner, backend.jobsRepo, backend.runID)
	}
	if detail.Kind != "workflow" || len(detail.Sections) != 1 || len(detail.Sections[0].Rows) != 3 {
		t.Fatalf("detail = %#v", detail)
	}
	if len(detail.Chips) != 1 || detail.Chips[0].Label != "in progress" {
		t.Fatalf("chips = %#v, want refreshed in progress status", detail.Chips)
	}
	if detail.Sections[0].Rows[0][0] != "test" || detail.Sections[0].Rows[2][0] != "  ↳ Run tests" {
		t.Fatalf("rows = %#v", detail.Sections[0].Rows)
	}
}
