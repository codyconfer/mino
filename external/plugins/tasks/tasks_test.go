package tasks

import (
	"testing"
	"time"

	tasksapi "google.golang.org/api/tasks/v1"
)

func TestTaskToItem(t *testing.T) {
	due := "2026-07-25T00:00:00Z"
	item := taskToItem(&tasksapi.Task{
		Title:       "write the report",
		Notes:       "cover Q3",
		Status:      "needsAction",
		Due:         due,
		WebViewLink: "https://tasks.google.com/task/abc",
	}, "Work")

	if item.Kind != "task" || item.Title != "write the report" || item.Subtitle != "Work" {
		t.Fatalf("mapped item = %+v", item)
	}
	if item.Body != "cover Q3" || item.URL != "https://tasks.google.com/task/abc" {
		t.Errorf("body/url = %q / %q", item.Body, item.URL)
	}
	if item.Meta["status"] != "needsAction" {
		t.Errorf("status meta = %q", item.Meta["status"])
	}
	want, _ := time.Parse(time.RFC3339, due)
	if !item.Timestamp.Equal(want) {
		t.Errorf("timestamp = %v, want %v", item.Timestamp, want)
	}
}

func TestTaskToItemFallsBackToUpdated(t *testing.T) {
	item := taskToItem(&tasksapi.Task{Title: "x", Updated: "2026-01-02T03:04:05Z"}, "L")
	if item.Timestamp.IsZero() {
		t.Error("expected updated to be used when due is empty")
	}
}

func TestNormalizeDue(t *testing.T) {
	got, err := normalizeDue("2026-07-25")
	if err != nil || got != "2026-07-25T00:00:00Z" {
		t.Fatalf("date normalize = %q, %v", got, err)
	}
	if got, err := normalizeDue("2026-07-25T09:30:00Z"); err != nil || got != "2026-07-25T09:30:00Z" {
		t.Fatalf("rfc3339 passthrough = %q, %v", got, err)
	}
	if _, err := normalizeDue("next tuesday"); err == nil {
		t.Error("expected error for invalid due")
	}
}
