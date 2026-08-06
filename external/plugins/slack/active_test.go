package slack

import (
	"testing"

	slackapi "github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

func TestActiveItemMetaContract(t *testing.T) {
	me := &slackevents.MessageEvent{
		Channel:         "C001",
		User:            "U1",
		Text:            "deploy done",
		TimeStamp:       "1700000000.000001",
		ThreadTimeStamp: "1699999999.000001",
	}
	it, ok := activeItem(me, itemCtx{channelID: "C001", host: "myorg.slack.com"})
	if !ok {
		t.Fatal("activeItem dropped a plain message")
	}
	if it.Meta["channel_id"] != "C001" {
		t.Errorf("Meta[channel_id] = %q", it.Meta["channel_id"])
	}
	if it.Meta["ts"] != "1700000000.000001" {
		t.Errorf("Meta[ts] = %q", it.Meta["ts"])
	}
	if it.Meta["thread_ts"] != "1699999999.000001" {
		t.Errorf("Meta[thread_ts] = %q, want the parent so Detail opens the thread", it.Meta["thread_ts"])
	}
	if it.Meta["author"] != "U1" {
		t.Errorf("Meta[author] = %q, want the raw id as a stable fallback", it.Meta["author"])
	}
	if it.URL == "" {
		t.Error("stream items should carry a permalink when the workspace host is known")
	}
}

func TestActiveItemSubtypes(t *testing.T) {
	base := func() *slackevents.MessageEvent {
		return &slackevents.MessageEvent{Channel: "C1", User: "U1", Text: "x", TimeStamp: "1700000000.000001"}
	}

	changed := base()
	changed.SubType = "message_changed"
	changed.Message = &slackapi.Msg{Text: "edited text", User: "U1", Timestamp: "1700000000.000002"}
	if it, ok := activeItem(changed, itemCtx{channelID: "C1"}); !ok || it.Kind != "edited" {
		t.Errorf("message_changed = (%+v, %v), want Kind edited", it, ok)
	}

	noPayload := base()
	noPayload.SubType = "message_changed"
	if _, ok := activeItem(noPayload, itemCtx{channelID: "C1"}); ok {
		t.Error("message_changed with no payload should be dropped")
	}

	deleted := base()
	deleted.SubType = "message_deleted"
	deleted.DeletedTimeStamp = "1700000000.000003"
	if it, ok := activeItem(deleted, itemCtx{channelID: "C1"}); !ok || it.Kind != "deleted" {
		t.Errorf("message_deleted = (%+v, %v), want Kind deleted", it, ok)
	}

	bot := base()
	bot.SubType = "bot_message"
	bot.User = ""
	bot.Username = "deploybot"
	if it, ok := activeItem(bot, itemCtx{channelID: "C1"}); !ok || it.Kind != "bot" || it.Meta["author"] != "deploybot" {
		t.Errorf("bot_message = (%+v, %v), want Kind bot authored by deploybot", it, ok)
	}

	unknown := base()
	unknown.SubType = "channel_join"
	if _, ok := activeItem(unknown, itemCtx{channelID: "C1"}); ok {
		t.Error("unhandled subtypes should be dropped")
	}
}

func TestActiveChannelAllowList(t *testing.T) {
	all := NewActive("bot", "app").(*activeSlack)
	if !all.allows("C001") {
		t.Error("an empty allow-list must pass everything, preserving today's behaviour")
	}

	only := NewActive("bot", "app", ActiveOptions{Channels: []string{"C001", "#eng"}}).(*activeSlack)
	if !only.allows("C001") {
		t.Error("C001 should be allowed")
	}
	if !only.allows("eng") {
		t.Error("a configured #eng should match the bare name")
	}
	if only.allows("C999") {
		t.Error("C999 is not in the allow-list")
	}
}

func TestActiveChannelSetIgnoresBlanks(t *testing.T) {
	a := NewActive("bot", "app", ActiveOptions{Channels: []string{"", "  "}}).(*activeSlack)
	if !a.allows("anything") {
		t.Error("a list of blanks should behave like no list at all")
	}
}
