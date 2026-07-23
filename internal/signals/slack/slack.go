package slack

import (
	"context"
	"strconv"
	"strings"
	"time"

	slackapi "github.com/slack-go/slack"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/signals"
)

const defaultLimit = 50

type slackSignal struct {
	token   string
	channel string
	limit   int
}

func New(token, channel string, limit int) signals.Signal {
	if limit <= 0 {
		limit = defaultLimit
	}
	return &slackSignal{token: token, channel: channel, limit: limit}
}

func (s *slackSignal) Name() string { return "slack" }

func (s *slackSignal) Fetch(ctx context.Context) ([]signals.Section, error) {
	api := slackapi.New(s.token)

	id, name, err := s.resolveChannel(ctx, api)
	if err != nil {
		return nil, err
	}

	resp, err := api.GetConversationHistoryContext(ctx, &slackapi.GetConversationHistoryParameters{
		ChannelID: id,
		Limit:     s.limit,
	})
	if err != nil {
		return nil, errs.Wrapf(errs.KindSignal, err, "slack: fetching history for %s", id)
	}

	title := "#" + name
	if name == "" {
		title = id
	}

	items := make([]signals.Item, 0, len(resp.Messages))
	for _, msg := range resp.Messages {
		items = append(items, messageToItem(msg, name))
	}

	return []signals.Section{{
		Signal: "slack",
		Title:  title,
		Items:  items,
	}}, nil
}

func (s *slackSignal) resolveChannel(ctx context.Context, api *slackapi.Client) (id, name string, err error) {
	want := strings.TrimPrefix(s.channel, "#")

	if isChannelID(s.channel) {
		return s.channel, "", nil
	}

	cursor := ""
	for {
		channels, next, listErr := api.GetConversationsContext(ctx, &slackapi.GetConversationsParameters{
			Types:  []string{"public_channel", "private_channel"},
			Cursor: cursor,
			Limit:  200,
		})
		if listErr != nil {
			return "", "", errs.Wrap(errs.KindSignal, listErr, "slack: listing conversations")
		}
		for _, ch := range channels {
			if ch.Name == want {
				return ch.ID, ch.Name, nil
			}
		}
		if next == "" {
			break
		}
		cursor = next
	}

	return "", "", errs.Newf(errs.KindSignal, "slack: channel %q not found", s.channel).
		WithHint("check the channel name, or use its ID (C.../G.../D...)")
}

func isChannelID(v string) bool {
	if v == "" {
		return false
	}
	switch v[0] {
	case 'C', 'G', 'D':
		return true
	default:
		return false
	}
}

func messageToItem(msg slackapi.Message, channelName string) signals.Item {
	title := firstLine(msg.Text)
	if title == "" {
		title = "(no text)"
	}
	title = capRunes(title, 120)

	subtitle := ""
	if channelName != "" {
		subtitle = "#" + channelName
	}

	return signals.Item{
		Kind:      "message",
		Title:     title,
		Subtitle:  subtitle,
		Body:      msg.Text,
		URL:       "",
		Timestamp: parseSlackTS(msg.Timestamp),
		Meta:      map[string]string{"user": msg.User},
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func capRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func parseSlackTS(ts string) time.Time {
	if ts == "" {
		return time.Time{}
	}
	secPart := ts
	if i := strings.IndexByte(ts, '.'); i >= 0 {
		secPart = ts[:i]
	}
	sec, err := strconv.ParseInt(secPart, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}
