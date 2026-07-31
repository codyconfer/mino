package drive

import (
	"testing"
	"time"

	driveapi "google.golang.org/api/drive/v3"
)

func TestFileToItem(t *testing.T) {
	mod := "2026-07-22T10:00:00Z"
	item := FileToItem(&driveapi.File{
		Id:           "abc",
		Name:         "notes.txt",
		MimeType:     "text/plain",
		ModifiedTime: mod,
		WebViewLink:  "https://drive.google.com/file/d/abc",
		Owners:       []*driveapi.User{{DisplayName: "Alice"}},
	})
	if item.Kind != "file" || item.Title != "notes.txt" {
		t.Fatalf("mapped item = %+v", item)
	}
	if item.Subtitle != "Alice" {
		t.Errorf("subtitle = %q, want Alice", item.Subtitle)
	}
	if item.URL != "https://drive.google.com/file/d/abc" {
		t.Errorf("url = %q", item.URL)
	}
	if item.Meta["mime"] != "text/plain" || item.Meta["id"] != "abc" {
		t.Errorf("meta = %v", item.Meta)
	}
	want, _ := time.Parse(time.RFC3339, mod)
	if !item.Timestamp.Equal(want) {
		t.Errorf("timestamp = %v, want %v", item.Timestamp, want)
	}
}

func TestFileToItemFallsBackToMime(t *testing.T) {
	item := FileToItem(&driveapi.File{Name: "x", MimeType: "application/pdf"})
	if item.Subtitle != "application/pdf" {
		t.Errorf("subtitle should fall back to mime, got %q", item.Subtitle)
	}
}
