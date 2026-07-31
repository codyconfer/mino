package gmail

import (
	"testing"
	"time"

	gmailapi "google.golang.org/api/gmail/v1"
)

func TestMessageToItem(t *testing.T) {
	const millis = 1_700_000_000_000
	msg := &gmailapi.Message{
		Id:           "abc123",
		Snippet:      "hello there, this is the body",
		InternalDate: millis,
		Payload: &gmailapi.MessagePart{
			Headers: []*gmailapi.MessagePartHeader{
				{Name: "subject", Value: "Weekly sync notes"},
				{Name: "From", Value: "Alice <alice@example.com>"},
			},
		},
	}

	item := messageToItem(msg)

	if item.Kind != "email" {
		t.Errorf("Kind = %q, want %q", item.Kind, "email")
	}
	if item.Title != "Weekly sync notes" {
		t.Errorf("Title = %q, want %q", item.Title, "Weekly sync notes")
	}
	if item.Subtitle != "Alice <alice@example.com>" {
		t.Errorf("Subtitle = %q, want %q", item.Subtitle, "Alice <alice@example.com>")
	}
	if item.Body != "hello there, this is the body" {
		t.Errorf("Body = %q, want %q", item.Body, "hello there, this is the body")
	}
	if want := time.UnixMilli(millis); !item.Timestamp.Equal(want) {
		t.Errorf("Timestamp = %v, want %v", item.Timestamp, want)
	}
	if item.URL != "https://mail.google.com/mail/#all/abc123" {
		t.Errorf("URL = %q, want %q", item.URL, "https://mail.google.com/mail/#all/abc123")
	}
	if item.Meta["from"] != "Alice <alice@example.com>" {
		t.Errorf("Meta[from] = %q, want %q", item.Meta["from"], "Alice <alice@example.com>")
	}
}

func TestMessageToItemDefaults(t *testing.T) {

	msg := &gmailapi.Message{
		Id:      "no-subject",
		Payload: &gmailapi.MessagePart{},
	}
	item := messageToItem(msg)
	if item.Title != "(no subject)" {
		t.Errorf("Title = %q, want %q", item.Title, "(no subject)")
	}
	if item.Subtitle != "" {
		t.Errorf("Subtitle = %q, want empty", item.Subtitle)
	}

	nilPayload := &gmailapi.Message{Id: "x"}
	got := messageToItem(nilPayload)
	if got.Title != "(no subject)" {
		t.Errorf("Title = %q, want %q", got.Title, "(no subject)")
	}
}
