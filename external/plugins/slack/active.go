package slack

import (
	"context"
	"time"

	slackapi "github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/codyconfer/munin/plugin"
)

type activeSlack struct {
	botToken string
	appToken string
}

func NewActive(botToken, appToken string) plugin.Stream {
	return &activeSlack{botToken: botToken, appToken: appToken}
}

func (h *activeSlack) Name() string { return "slack" }

func (h *activeSlack) LatencyFloor() time.Duration { return 0 }

func (h *activeSlack) Stream(ctx context.Context) (<-chan plugin.Event, error) {
	api := slackapi.New(h.botToken, slackapi.OptionAppLevelToken(h.appToken))
	sm := socketmode.New(api)

	out := make(chan plugin.Event)

	go func() {
		_ = sm.RunContext(ctx)
	}()

	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-sm.Events:
				if !ok {
					return
				}
				if evt.Type != socketmode.EventTypeEventsAPI {
					continue
				}
				if evt.Request != nil {
					_ = sm.Ack(*evt.Request)
				}
				api, ok := evt.Data.(slackevents.EventsAPIEvent)
				if !ok {
					continue
				}
				me, ok := api.InnerEvent.Data.(*slackevents.MessageEvent)
				if !ok {
					continue
				}
				it, ok := activeItem(me)
				if !ok {
					continue
				}
				ev := plugin.Event{
					Source: "slack",
					At:     time.Now(),
					Section: plugin.Section{
						Signal: "slack",
						Title:  "slack",
						Items:  []plugin.Item{it},
					},
				}
				select {
				case <-ctx.Done():
					return
				case out <- ev:
				}
			}
		}
	}()

	return out, nil
}

func activeItem(me *slackevents.MessageEvent) (plugin.Item, bool) {
	switch me.SubType {
	case "", "thread_broadcast":
		it := messageToItem(slackapi.Message{Msg: slackapi.Msg{
			Text:      me.Text,
			User:      me.User,
			Timestamp: me.TimeStamp,
		}}, me.Channel)
		it.Subtitle = me.Channel
		return it, true
	case "message_changed":
		if me.Message == nil {
			return plugin.Item{}, false
		}
		it := messageToItem(slackapi.Message{Msg: *me.Message}, me.Channel)
		it.Kind = "edited"
		it.Subtitle = me.Channel
		return it, true
	case "message_deleted":
		it := messageToItem(slackapi.Message{Msg: slackapi.Msg{
			Text:      "(message deleted)",
			Timestamp: me.DeletedTimeStamp,
		}}, me.Channel)
		it.Kind = "deleted"
		it.Subtitle = me.Channel
		return it, true
	case "bot_message":
		user := me.Username
		if user == "" {
			user = me.BotID
		}
		it := messageToItem(slackapi.Message{Msg: slackapi.Msg{
			Text:      me.Text,
			User:      user,
			Timestamp: me.TimeStamp,
		}}, me.Channel)
		it.Kind = "bot"
		it.Subtitle = me.Channel
		return it, true
	default:
		return plugin.Item{}, false
	}
}
