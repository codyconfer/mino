package slack

import (
	"context"
	"strings"
	"time"

	slackapi "github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/codyconfer/mino/plugin"
)

type ActiveOptions struct {
	Channels  []string
	Workspace string
}

type activeSlack struct {
	botToken string
	appToken string
	channels map[string]bool
	host     string
}

func NewActive(botToken, appToken string, opts ...ActiveOptions) plugin.Stream {
	a := &activeSlack{botToken: botToken, appToken: appToken}
	for _, o := range opts {
		a.host = o.Workspace
		a.channels = channelSet(o.Channels)
	}
	return a
}

func channelSet(list []string) map[string]bool {
	if len(list) == 0 {
		return nil
	}
	out := make(map[string]bool, len(list))
	for _, v := range list {
		v = strings.TrimPrefix(strings.TrimSpace(v), "#")
		if v != "" {
			out[v] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (h *activeSlack) Name() string { return SignalName }

func (h *activeSlack) LatencyFloor() time.Duration { return 0 }

func (h *activeSlack) allows(channel string) bool {
	if len(h.channels) == 0 {
		return true
	}
	return h.channels[channel]
}

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
				if !h.allows(me.Channel) {
					continue
				}
				it, ok := activeItem(me, itemCtx{channelID: me.Channel, host: h.host})
				if !ok {
					continue
				}
				ev := plugin.Event{
					Source: SignalName,
					At:     time.Now(),
					Section: plugin.Section{
						Signal: SignalName,
						Title:  SignalName,
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

func activeItem(me *slackevents.MessageEvent, c itemCtx) (plugin.Item, bool) {
	if c.channelID == "" {
		c.channelID = me.Channel
	}
	switch me.SubType {
	case "", "thread_broadcast":
		return messageToItem(slackapi.Message{Msg: slackapi.Msg{
			Text:            me.Text,
			User:            me.User,
			Timestamp:       me.TimeStamp,
			ThreadTimestamp: me.ThreadTimeStamp,
			SubType:         me.SubType,
		}}, c), true
	case "message_changed":
		if me.Message == nil {
			return plugin.Item{}, false
		}
		c.kind = "edited"
		return messageToItem(slackapi.Message{Msg: *me.Message}, c), true
	case "message_deleted":
		c.kind = "deleted"
		return messageToItem(slackapi.Message{Msg: slackapi.Msg{
			Text:      "(message deleted)",
			Timestamp: me.DeletedTimeStamp,
			SubType:   me.SubType,
		}}, c), true
	case "bot_message":
		c.kind = "bot"
		return messageToItem(slackapi.Message{Msg: slackapi.Msg{
			Text:      me.Text,
			Username:  me.Username,
			BotID:     me.BotID,
			Timestamp: me.TimeStamp,
			SubType:   me.SubType,
		}}, c), true
	default:
		return plugin.Item{}, false
	}
}
