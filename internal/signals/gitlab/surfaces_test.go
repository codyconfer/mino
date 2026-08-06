package gitlab

import (
	"encoding/json"
	"testing"

	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/internal/signals"
)

const mrFixture = `[
  {
    "id": 900, "iid": 42, "project_id": 7,
    "title": "Drain the payments queue", "description": "body text",
    "state": "opened", "draft": false,
    "web_url": "https://gitlab.com/acme/platform/api/-/merge_requests/42",
    "updated_at": "2026-08-04T10:00:00Z", "created_at": "2026-08-01T10:00:00Z",
    "source_branch": "fix/queue", "target_branch": "main",
    "labels": ["backend", "urgent"],
    "merge_status": "can_be_merged", "detailed_merge_status": "ci_must_pass",
    "changes_count": "12",
    "author": {"username": "codyconfer"},
    "assignees": [{"username": "codyconfer"}],
    "reviewers": [{"username": "reviewer1"}, {"username": "reviewer2"}],
    "milestone": {"title": "24.9"},
    "references": {"full": "acme/platform/api!42"}
  },
  {
    "id": 901, "iid": 43, "project_id": 7,
    "title": "Draft work", "state": "opened", "draft": true,
    "web_url": "https://gitlab.com/acme/platform/api/-/merge_requests/43",
    "updated_at": "2026-08-03T10:00:00Z",
    "author": {"username": "someone"}
  }
]`

const issueFixture = `[
  {
    "id": 500, "iid": 7, "project_id": 7,
    "title": "Flaky integration test", "description": "sometimes red",
    "state": "opened", "confidential": true, "due_date": "2026-09-01", "weight": 3,
    "web_url": "https://gitlab.com/acme/platform/api/-/issues/7",
    "updated_at": "2026-08-02T10:00:00Z",
    "labels": ["test"],
    "author": {"username": "codyconfer"},
    "assignees": [{"username": "codyconfer"}],
    "milestone": {"title": "24.9"},
    "references": {"full": "acme/platform/api#7"}
  }
]`

const pipelineFixture = `[
  {
    "id": 90210, "iid": 12, "project_id": 7,
    "name": "nightly", "status": "failed", "source": "schedule",
    "ref": "main", "sha": "abcdef0123456789",
    "web_url": "https://gitlab.com/acme/platform/api/-/pipelines/90210",
    "updated_at": "2026-08-05T09:00:00Z"
  },
  {
    "id": 90211, "project_id": 7, "status": "running", "ref": "topic",
    "web_url": "https://gitlab.com/acme/platform/api/-/pipelines/90211",
    "updated_at": "2026-08-05T09:05:00Z"
  }
]`

func decodeMRs(t *testing.T) []mergeRequestWire {
	t.Helper()
	var rows []mergeRequestWire
	if err := json.Unmarshal([]byte(mrFixture), &rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestMergeRequestMapping(t *testing.T) {
	it := decodeMRs(t)[0].item()

	if it.Kind != "mr" {
		t.Errorf("kind = %q, want mr", it.Kind)
	}
	if it.Title != "Drain the payments queue" {
		t.Errorf("title = %q", it.Title)
	}
	if it.Subtitle != "acme/platform/api" {
		t.Errorf("subtitle = %q, want the project path from references.full", it.Subtitle)
	}
	if it.Body != "body text" || it.URL == "" {
		t.Errorf("body/url = %q/%q", it.Body, it.URL)
	}
	if it.Timestamp.IsZero() {
		t.Error("timestamp was not parsed from updated_at")
	}

	want := map[string]string{
		"state":                 "opened",
		"iid":                   "42",
		"id":                    "900",
		"project":               "acme/platform/api",
		"author":                "codyconfer",
		"source_branch":         "fix/queue",
		"target_branch":         "main",
		"branches":              "fix/queue -> main",
		"labels":                "backend,urgent",
		"assignees":             "@codyconfer",
		"reviewers":             "@reviewer1,@reviewer2",
		"milestone":             "24.9",
		"merge_status":          "can_be_merged",
		"detailed_merge_status": "ci_must_pass",
		"changes_count":         "12",
		"updated":               "2026-08-04T10:00:00Z",
	}
	for k, v := range want {
		if it.Meta[k] != v {
			t.Errorf("meta[%q] = %q, want %q", k, it.Meta[k], v)
		}
	}
	if _, ok := it.Meta["draft"]; ok {
		t.Error("a non-draft MR carries a draft key")
	}
}

func TestMergeRequestKeepsGitLabsOwnStateWord(t *testing.T) {
	it := decodeMRs(t)[0].item()
	if it.Meta["state"] != "opened" {
		t.Errorf("state = %q, want GitLab's own \"opened\"; rewriting it to \"open\" would make JSON "+
			"output and user filter rules disagree with the API", it.Meta["state"])
	}
	if got := signals.ClassifyItem(it); got != glyph.SeverityNeutral {
		t.Errorf("severity = %v, want neutral for an open MR", got)
	}
}

func TestMergeRequestFallsBackToTheWebURLForTheProject(t *testing.T) {
	var w mergeRequestWire
	if err := json.Unmarshal([]byte(`{
	  "iid": 9, "web_url": "https://gitlab.com/acme/platform/sub/api/-/merge_requests/9",
	  "state": "opened"
	}`), &w); err != nil {
		t.Fatal(err)
	}
	if got := w.item().Subtitle; got != "acme/platform/sub/api" {
		t.Errorf("subtitle = %q, want the nested path recovered from web_url when references are "+
			"absent", got)
	}
}

func TestMergeRequestDraftIsFlagged(t *testing.T) {
	if got := decodeMRs(t)[1].item().Meta["draft"]; got != "true" {
		t.Errorf("draft = %q, want true", got)
	}
}

func TestIssueMapping(t *testing.T) {
	var rows []issueWire
	if err := json.Unmarshal([]byte(issueFixture), &rows); err != nil {
		t.Fatal(err)
	}
	it := rows[0].item()

	if it.Kind != "issue" || it.Subtitle != "acme/platform/api" {
		t.Errorf("kind/subtitle = %q/%q", it.Kind, it.Subtitle)
	}
	for k, v := range map[string]string{
		"state":        "opened",
		"iid":          "7",
		"confidential": "true",
		"due_date":     "2026-09-01",
		"weight":       "3",
		"labels":       "test",
		"assignees":    "@codyconfer",
		"milestone":    "24.9",
	} {
		if it.Meta[k] != v {
			t.Errorf("meta[%q] = %q, want %q", k, it.Meta[k], v)
		}
	}
}

func TestClosedIssueIsPositiveAndClosedMRIsNegative(t *testing.T) {
	issue := signals.Item{Kind: "issue", Meta: map[string]string{"state": "closed"}}
	if got := signals.ClassifyItem(issue); got != glyph.SeverityPositive {
		t.Errorf("closed issue = %v, want positive", got)
	}
	mr := signals.Item{Kind: "mr", Meta: map[string]string{"state": "closed"}}
	if got := signals.ClassifyItem(mr); got != glyph.SeverityNegative {
		t.Errorf("closed MR = %v, want negative; an MR closed without merging is a loss", got)
	}
}

func TestPipelineMapping(t *testing.T) {
	var rows []pipelineWire
	if err := json.Unmarshal([]byte(pipelineFixture), &rows); err != nil {
		t.Fatal(err)
	}

	first := rows[0].item("acme/platform/api")
	if first.Kind != "pipeline" {
		t.Errorf("kind = %q", first.Kind)
	}
	if first.Title != "nightly #12" {
		t.Errorf("title = %q, want the pipeline name and iid", first.Title)
	}
	if first.Subtitle != "acme/platform/api · main" {
		t.Errorf("subtitle = %q, want project and ref", first.Subtitle)
	}
	if first.Meta["pipeline_id"] != "90210" {
		t.Errorf("pipeline_id = %q, want the global id 90210; /pipelines/:id/jobs takes that, not the "+
			"iid, and using the iid silently fetches another pipeline's jobs", first.Meta["pipeline_id"])
	}
	if first.Meta["iid"] != "12" {
		t.Errorf("iid = %q, want 12", first.Meta["iid"])
	}
	for k, v := range map[string]string{"status": "failed", "ref": "main", "source": "schedule"} {
		if first.Meta[k] != v {
			t.Errorf("meta[%q] = %q, want %q", k, first.Meta[k], v)
		}
	}

	second := rows[1].item("acme/platform/api")
	if second.Title != "pipeline #90211" {
		t.Errorf("unnamed pipeline title = %q, want it to fall back to the global id", second.Title)
	}
}

func TestPipelineSeverityCoversGitLabsStatuses(t *testing.T) {
	cases := map[string]glyph.Severity{
		"success":              glyph.SeverityPositive,
		"failed":               glyph.SeverityNegative,
		"running":              glyph.SeverityWarning,
		"pending":              glyph.SeverityWarning,
		"created":              glyph.SeverityWarning,
		"preparing":            glyph.SeverityWarning,
		"scheduled":            glyph.SeverityWarning,
		"manual":               glyph.SeverityWarning,
		"waiting_for_resource": glyph.SeverityWarning,
		"canceled":             glyph.SeverityNeutral,
		"skipped":              glyph.SeverityNeutral,
	}
	for status, want := range cases {
		if got := pipelineSeverity(status); got != want {
			t.Errorf("pipelineSeverity(%q) = %v, want %v", status, got, want)
		}
		item := signals.Item{Kind: "pipeline", Meta: map[string]string{"status": status}}
		if got := signals.ClassifyItem(item); got != want {
			t.Errorf("ClassifyItem(pipeline %q) = %v, want %v", status, got, want)
		}
	}
}

func TestJobInProgress(t *testing.T) {
	for _, s := range []string{"running", "pending", "created", "manual"} {
		if !jobInProgress(s) {
			t.Errorf("jobInProgress(%q) = false", s)
		}
	}
	for _, s := range []string{"success", "failed", "canceled", "skipped"} {
		if jobInProgress(s) {
			t.Errorf("jobInProgress(%q) = true", s)
		}
	}
}
