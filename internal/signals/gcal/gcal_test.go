package gcal

import (
	"testing"
	"time"

	calendar "google.golang.org/api/calendar/v3"
)

func TestEventToItem_Timed(t *testing.T) {
	ev := &calendar.Event{
		Summary:     "Standup",
		Location:    "Zoom",
		Description: "daily sync",
		HtmlLink:    "https://calendar.google.com/event?eid=1",
		Organizer:   &calendar.EventOrganizer{Email: "lead@example.com"},
		Start:       &calendar.EventDateTime{DateTime: "2026-07-22T09:30:00-04:00"},
	}

	got := eventToItem(ev)

	if got.Kind != "event" {
		t.Errorf("Kind = %q, want %q", got.Kind, "event")
	}
	if got.Title != "Standup" {
		t.Errorf("Title = %q, want %q", got.Title, "Standup")
	}
	if got.Subtitle != "Zoom" {
		t.Errorf("Subtitle = %q, want %q", got.Subtitle, "Zoom")
	}
	if got.Body != "daily sync" {
		t.Errorf("Body = %q, want %q", got.Body, "daily sync")
	}
	if got.URL != "https://calendar.google.com/event?eid=1" {
		t.Errorf("URL = %q", got.URL)
	}
	if got.Meta["organizer"] != "lead@example.com" {
		t.Errorf("Meta[organizer] = %q, want %q", got.Meta["organizer"], "lead@example.com")
	}
	want, _ := time.Parse(time.RFC3339, "2026-07-22T09:30:00-04:00")
	if !got.Timestamp.Equal(want) {
		t.Errorf("Timestamp = %v, want %v", got.Timestamp, want)
	}
}

func TestEventToItem_AllDay(t *testing.T) {
	ev := &calendar.Event{
		Summary: "Company holiday",
		Start:   &calendar.EventDateTime{Date: "2026-12-25"},
	}

	got := eventToItem(ev)

	if got.Title != "Company holiday" {
		t.Errorf("Title = %q, want %q", got.Title, "Company holiday")
	}
	if got.Subtitle != "" {
		t.Errorf("Subtitle = %q, want empty", got.Subtitle)
	}
	if got.Meta != nil {
		t.Errorf("Meta = %v, want nil (no organizer)", got.Meta)
	}
	want, _ := time.Parse("2006-01-02", "2026-12-25")
	if !got.Timestamp.Equal(want) {
		t.Errorf("Timestamp = %v, want %v", got.Timestamp, want)
	}
}

func TestEventToItem_NilStart(t *testing.T) {
	ev := &calendar.Event{Summary: "No time"}

	got := eventToItem(ev)

	if got.Title != "No time" {
		t.Errorf("Title = %q, want %q", got.Title, "No time")
	}
	if !got.Timestamp.IsZero() {
		t.Errorf("Timestamp = %v, want zero", got.Timestamp)
	}
}
