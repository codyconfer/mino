package gdocs

import (
	"testing"
	"time"

	drive "google.golang.org/api/drive/v3"
)

func TestFileToItem(t *testing.T) {
	f := &drive.File{
		Name:         "Design Doc",
		ModifiedTime: "2026-07-22T15:04:05Z",
		WebViewLink:  "https://docs.google.com/document/d/abc123/edit",
		Owners: []*drive.User{
			{DisplayName: "Ada Lovelace"},
		},
	}

	item := fileToItem(f)

	if item.Kind != "doc" {
		t.Errorf("Kind = %q, want %q", item.Kind, "doc")
	}
	if item.Title != "Design Doc" {
		t.Errorf("Title = %q, want %q", item.Title, "Design Doc")
	}
	if item.Subtitle != "Ada Lovelace" {
		t.Errorf("Subtitle = %q, want %q", item.Subtitle, "Ada Lovelace")
	}
	if item.URL != "https://docs.google.com/document/d/abc123/edit" {
		t.Errorf("URL = %q, want the webViewLink", item.URL)
	}

	want, _ := time.Parse(time.RFC3339, "2026-07-22T15:04:05Z")
	if !item.Timestamp.Equal(want) {
		t.Errorf("Timestamp = %v, want %v", item.Timestamp, want)
	}
}

func TestFileToItemNoOwners(t *testing.T) {
	f := &drive.File{
		Name:        "Orphan Doc",
		WebViewLink: "https://docs.google.com/document/d/xyz/edit",
	}

	item := fileToItem(f)

	if item.Subtitle != "" {
		t.Errorf("Subtitle = %q, want empty for no owners", item.Subtitle)
	}
	if !item.Timestamp.IsZero() {
		t.Errorf("Timestamp = %v, want zero for missing modifiedTime", item.Timestamp)
	}
	if item.Meta == nil {
		t.Error("Meta = nil, want non-nil empty map")
	}
}
