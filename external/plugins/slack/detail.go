package slack

import (
	"context"
	"strconv"
	"strings"

	"github.com/codyconfer/viewkit/glyph"
	slackapi "github.com/slack-go/slack"

	"github.com/codyconfer/mino/external/plugins/internal/errx"
	"github.com/codyconfer/mino/plugin"
)

const (
	detailReplies = 30
	detailChips   = 8
)

func (s *slackSignal) Detail(ctx context.Context, it plugin.Item) (plugin.ItemDetail, error) {
	ref, err := refFromItem(it)
	if err != nil {
		return plugin.ItemDetail{}, err
	}

	api := s.client()
	who := s.identify(ctx, api)

	msgs, _, _, err := api.GetConversationRepliesContext(ctx, &slackapi.GetConversationRepliesParameters{
		ChannelID: ref.ChannelID,
		Timestamp: ref.root(),
		Limit:     detailReplies,
	})
	if err != nil {
		return plugin.ItemDetail{}, errx.Wrapf(scopeError(err, "reading a thread"),
			"slack: fetching thread %s in %s", ref.root(), ref.ChannelID)
	}
	if len(msgs) == 0 {
		return plugin.ItemDetail{}, errx.Newf("slack: message %s not found in %s", ref.TS, ref.ChannelID).
			WithHint("the message may have been deleted, or the token cannot see that channel")
	}

	names := s.resolveUsers(ctx, api, messageUserIDs(msgs))
	return detailFromThread(msgs, ref, it, names, who.host), nil
}

func refFromItem(it plugin.Item) (msgRef, error) {
	ref := msgRef{
		ChannelID: it.Meta["channel_id"],
		TS:        it.Meta["ts"],
		ThreadTS:  it.Meta["thread_ts"],
	}
	if ref.ok() {
		return ref, nil
	}
	if parsed, ok := parsePermalink(it.URL); ok {
		return parsed, nil
	}
	return msgRef{}, errx.New("slack: this item carries no channel and timestamp").
		WithHint("pass a message permalink (https://<workspace>/archives/<channel>/p<ts>), or re-run the query to refresh cached items")
}

func detailFromThread(msgs []slackapi.Message, ref msgRef, it plugin.Item, names map[string]string, host string) plugin.ItemDetail {
	root := msgs[0]
	replies := msgs[1:]

	channelName := it.Meta["channel_name"]
	rootCtx := itemCtx{
		channelID:   ref.ChannelID,
		channelName: channelName,
		host:        host,
		names:       names,
	}
	rootItem := messageToItem(root, rootCtx)

	title := rootItem.Title
	if title == "" {
		title = it.Title
	}

	channelLabel := channelName
	if channelLabel == "" {
		channelLabel = ref.ChannelID
	} else {
		channelLabel = "#" + channelLabel
	}

	rows := [][2]string{
		{"author", authorOf(root, names)},
		{"channel", channelLabel},
	}
	if posted := parseSlackTS(root.Timestamp); !posted.IsZero() {
		rows = append(rows, [2]string{"posted", posted.Format("2006-01-02 15:04")})
	}
	if n := replyCount(root, replies); n > 0 {
		rows = append(rows, [2]string{"replies", strconv.Itoa(n)})
	}
	if line := reactionLine(root.Reactions); line != "" {
		rows = append(rows, [2]string{"reactions", line})
	}

	detail := plugin.ItemDetail{
		Kind:  "message",
		Title: title,
		URL:   permalink(host, msgRef{ChannelID: ref.ChannelID, TS: root.Timestamp}),
		Chips: threadChips(root, replies),
		Rows:  rows,
		Body:  unfurl(root.Text, names),
		Meta:  rootItem.Meta,
	}
	if sec, ok := repliesSection(replies, ref, names, host); ok {
		detail.Sections = append(detail.Sections, sec)
	}
	if sec, ok := reactionsSection(root.Reactions); ok {
		detail.Sections = append(detail.Sections, sec)
	}
	return detail
}

func replyCount(root slackapi.Message, replies []slackapi.Message) int {
	if root.ReplyCount > 0 {
		return root.ReplyCount
	}
	return len(replies)
}

func threadChips(root slackapi.Message, replies []slackapi.Message) []plugin.Chip {
	var chips []plugin.Chip
	for i, r := range root.Reactions {
		if i >= detailChips {
			break
		}
		if r.Name == "" {
			continue
		}
		chips = append(chips, plugin.Chip{
			Label: ":" + r.Name + ": " + strconv.Itoa(r.Count),
			Sev:   glyph.SeverityNeutral,
		})
	}
	if n := replyCount(root, replies); n > 0 {
		label := strconv.Itoa(n) + " replies"
		if n == 1 {
			label = "1 reply"
		}
		chips = append(chips, plugin.Chip{Label: label, Sev: glyph.SeverityPositive})
	}
	if root.SubType == "bot_message" || root.BotID != "" {
		chips = append(chips, plugin.Chip{Label: "bot", Sev: glyph.SeverityWarning})
	}
	return chips
}

func repliesSection(replies []slackapi.Message, ref msgRef, names map[string]string, host string) (plugin.DetailSection, bool) {
	if len(replies) == 0 {
		return plugin.DetailSection{}, false
	}
	lines := make([]string, 0, len(replies))
	for _, r := range replies {
		var b strings.Builder
		b.WriteString(authorOf(r, names))
		if at := parseSlackTS(r.Timestamp); !at.IsZero() {
			b.WriteString("  ")
			b.WriteString(at.Format("15:04"))
		}
		b.WriteString("  ")
		b.WriteString(capRunes(firstLine(unfurl(r.Text, names)), titleCap))
		lines = append(lines, b.String())
	}
	title := "thread (" + strconv.Itoa(len(replies)) + ")"
	return plugin.DetailSection{
		Title: title,
		Lines: lines,
		Meta:  map[string]string{"channel_id": ref.ChannelID, "thread_ts": ref.root()},
	}, true
}

func reactionsSection(rs []slackapi.ItemReaction) (plugin.DetailSection, bool) {
	if len(rs) <= detailChips {
		return plugin.DetailSection{}, false
	}
	lines := make([]string, 0, len(rs))
	for _, r := range rs {
		if r.Name == "" {
			continue
		}
		lines = append(lines, ":"+r.Name+": "+strconv.Itoa(r.Count))
	}
	return plugin.DetailSection{Title: "reactions", Lines: lines}, true
}
