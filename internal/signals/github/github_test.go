package github

import (
	"testing"
	"time"
)

const searchFixture = `{
  "total_count": 2,
  "incomplete_results": false,
  "items": [
    {
      "title": "Add retry to fetcher",
      "html_url": "https://github.com/octo/munin/pull/7",
      "body": "Retries transient failures.",
      "updated_at": "2026-07-20T15:04:05Z",
      "repository_url": "https://api.github.com/repos/octo/munin",
      "user": {"login": "alice"}
    },
    {
      "title": "Fix flaky test",
      "html_url": "https://github.com/octo/tools/pull/12",
      "body": "Stabilize the timing assertion.",
      "updated_at": "2026-07-21T09:30:00Z",
      "repository_url": "https://api.github.com/repos/octo/tools",
      "user": {"login": "bob"}
    }
  ]
}`

func TestMapSearchResponse(t *testing.T) {
	sec, err := mapSearchResponse([]byte(searchFixture), "Open Pull Requests")
	if err != nil {
		t.Fatalf("mapSearchResponse: %v", err)
	}
	if sec.Signal != "github" {
		t.Errorf("Signal = %q, want %q", sec.Signal, "github")
	}
	if sec.Title != "Open Pull Requests" {
		t.Errorf("Title = %q, want %q", sec.Title, "Open Pull Requests")
	}
	if len(sec.Items) != 2 {
		t.Fatalf("got %d items, want 2", len(sec.Items))
	}

	cases := []struct {
		title    string
		subtitle string
		author   string
		url      string
		ts       string
	}{
		{"Add retry to fetcher", "octo/munin", "alice", "https://github.com/octo/munin/pull/7", "2026-07-20T15:04:05Z"},
		{"Fix flaky test", "octo/tools", "bob", "https://github.com/octo/tools/pull/12", "2026-07-21T09:30:00Z"},
	}
	for i, want := range cases {
		got := sec.Items[i]
		if got.Kind != "pr" {
			t.Errorf("item %d Kind = %q, want %q", i, got.Kind, "pr")
		}
		if got.Title != want.title {
			t.Errorf("item %d Title = %q, want %q", i, got.Title, want.title)
		}
		if got.Subtitle != want.subtitle {
			t.Errorf("item %d Subtitle = %q, want %q", i, got.Subtitle, want.subtitle)
		}
		if got.URL != want.url {
			t.Errorf("item %d URL = %q, want %q", i, got.URL, want.url)
		}
		if got.Meta["author"] != want.author {
			t.Errorf("item %d author = %q, want %q", i, got.Meta["author"], want.author)
		}
		wantTS, _ := time.Parse(time.RFC3339, want.ts)
		if !got.Timestamp.Equal(wantTS) {
			t.Errorf("item %d Timestamp = %v, want %v", i, got.Timestamp, wantTS)
		}
	}
}
