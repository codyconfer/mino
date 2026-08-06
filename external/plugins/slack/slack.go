package slack

import (
	"context"
	"strconv"
	"strings"

	slackapi "github.com/slack-go/slack"

	"github.com/codyconfer/mino/external/plugins/internal/errx"
	"github.com/codyconfer/mino/plugin"
)

const (
	defaultLimit = 50
	listPageSize = 200
	maxListPages = 50
	maxChannels  = 8
)

type Spec struct {
	Token        string
	APIURL       string
	Channels     []string
	Mentions     bool
	MentionQuery string
	DMs          int
	Search       string
	Limit        int
	ResolveNames bool
	Workspace    string
	RetryMax     int
}

type slackSignal struct {
	token        string
	apiURL       string
	limit        int
	retryMax     int
	resolveNames bool
	workspace    string
	channels     []string
	mentions     bool
	mentionQuery string
	dms          int
	search       string
}

type itemCtx struct {
	channelID   string
	channelName string
	host        string
	names       map[string]string
	kind        string
}

type surface struct {
	title string
	fetch func(ctx context.Context, api *slackapi.Client, who whoami) (plugin.Section, error)
}

func NewSpec(sp Spec) plugin.Query {
	limit := sp.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	channels := sp.Channels
	if len(channels) > maxChannels {
		channels = channels[:maxChannels]
	}
	return &slackSignal{
		token:        sp.Token,
		apiURL:       sp.APIURL,
		limit:        limit,
		retryMax:     sp.RetryMax,
		resolveNames: sp.ResolveNames,
		workspace:    sp.Workspace,
		channels:     channels,
		mentions:     sp.Mentions,
		mentionQuery: sp.MentionQuery,
		dms:          sp.DMs,
		search:       sp.Search,
	}
}

func New(token, channel string, limit int) plugin.Query {
	return NewSpec(Spec{
		Token:        token,
		Channels:     ChannelList(channel),
		Limit:        limit,
		ResolveNames: true,
		RetryMax:     retryMaxDefault,
	})
}

func ChannelList(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		v := strings.TrimSpace(part)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func (s *slackSignal) Name() string { return SignalName }

func (s *slackSignal) client() *slackapi.Client {
	return clientFor(s.token, s.apiURL, s.retryMax)
}

func (s *slackSignal) surfaces() []surface {
	var out []surface
	for _, ch := range s.channels {
		channel := ch
		title := channel
		if !isChannelID(channel) {
			title = "#" + strings.TrimPrefix(channel, "#")
		}
		out = append(out, surface{
			title: title,
			fetch: func(ctx context.Context, api *slackapi.Client, who whoami) (plugin.Section, error) {
				return s.fetchChannel(ctx, api, who, channel)
			},
		})
	}
	if s.mentions {
		out = append(out, surface{title: "mentions", fetch: s.fetchMentions})
	}
	if s.dms > 0 {
		out = append(out, surface{title: "dms", fetch: s.fetchDMs})
	}
	if s.search != "" {
		out = append(out, surface{
			title: "search: " + capRunes(s.search, 60),
			fetch: s.fetchSearch,
		})
	}
	return out
}

func (s *slackSignal) Fetch(ctx context.Context) ([]plugin.Section, error) {
	surfaces := s.surfaces()
	if len(surfaces) == 0 {
		return nil, errx.New("slack: nothing to read").
			WithHint("pass --channel, --mentions, --dms or --search, or set `plugins.slack.channel`")
	}
	api := s.client()
	who := s.identify(ctx, api)

	out := make([]plugin.Section, 0, len(surfaces))
	for _, sf := range surfaces {
		sec, err := sf.fetch(ctx, api, who)
		if err != nil {
			if len(surfaces) == 1 {
				return nil, err
			}
			out = append(out, plugin.Section{Signal: SignalName, Title: sf.title, Err: err})
			continue
		}
		out = append(out, sec)
	}
	return out, nil
}

func (s *slackSignal) identify(ctx context.Context, api *slackapi.Client) whoami {
	who := whoami{host: s.workspace}
	if who.host != "" && !s.mentions {
		return who
	}
	got, err := s.me(ctx, api)
	if err != nil {
		return who
	}
	who.userID = got.userID
	if who.host == "" {
		who.host = got.host
	}
	return who
}

func (s *slackSignal) fetchChannel(ctx context.Context, api *slackapi.Client, who whoami, channel string) (plugin.Section, error) {
	id, name, err := s.resolveChannel(ctx, api, channel)
	if err != nil {
		return plugin.Section{}, err
	}

	resp, err := api.GetConversationHistoryContext(ctx, &slackapi.GetConversationHistoryParameters{
		ChannelID: id,
		Limit:     s.limit,
	})
	if err != nil {
		return plugin.Section{}, errx.Wrapf(scopeError(err, "reading channel history"), "slack: fetching history for %s", id)
	}

	title := "#" + name
	if name == "" {
		title = id
	}

	c := itemCtx{
		channelID:   id,
		channelName: name,
		host:        who.host,
		names:       s.resolveUsers(ctx, api, messageUserIDs(resp.Messages)),
	}
	items := make([]plugin.Item, 0, len(resp.Messages))
	for _, msg := range resp.Messages {
		items = append(items, messageToItem(msg, c))
	}
	return plugin.Section{Signal: SignalName, Title: title, Items: items}, nil
}

func messageUserIDs(msgs []slackapi.Message) []string {
	seen := map[string]bool{}
	for _, m := range msgs {
		if m.User != "" {
			seen[m.User] = true
		}
		mentionIDs(m.Text, seen)
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	return out
}

func messageToItem(msg slackapi.Message, c itemCtx) plugin.Item {
	ref := msgRef{ChannelID: c.channelID, TS: msg.Timestamp, ThreadTS: msg.ThreadTimestamp}

	text := unfurl(msg.Text, c.names)
	title := firstLine(text)
	if title == "" {
		title = "(no text)"
	}
	title = capRunes(title, titleCap)

	subtitle := ""
	if c.channelName != "" {
		subtitle = "#" + c.channelName
	} else if c.channelID != "" {
		subtitle = c.channelID
	}

	kind := c.kind
	if kind == "" {
		kind = "message"
	}

	meta := map[string]string{}
	putMeta(meta, "channel_id", c.channelID)
	putMeta(meta, "channel_name", c.channelName)
	putMeta(meta, "ts", msg.Timestamp)
	putMeta(meta, "thread_ts", msg.ThreadTimestamp)
	putMeta(meta, "user", msg.User)
	putMeta(meta, "author", authorOf(msg, c.names))
	putMeta(meta, "subtype", msg.SubType)
	if msg.ReplyCount > 0 {
		putMeta(meta, "reply_count", strconv.Itoa(msg.ReplyCount))
	}
	putMeta(meta, "reactions", reactionLine(msg.Reactions))

	return plugin.Item{
		Kind:      kind,
		Title:     title,
		Subtitle:  subtitle,
		Body:      text,
		URL:       permalink(c.host, ref),
		Timestamp: parseSlackTS(msg.Timestamp),
		Meta:      meta,
	}
}

func authorOf(msg slackapi.Message, names map[string]string) string {
	if name, ok := names[msg.User]; ok && name != "" {
		return name
	}
	if msg.User != "" {
		return msg.User
	}
	if msg.Username != "" {
		return msg.Username
	}
	return msg.BotID
}

func putMeta(m map[string]string, key, value string) {
	if value != "" {
		m[key] = value
	}
}

func reactionLine(rs []slackapi.ItemReaction) string {
	if len(rs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(rs))
	for _, r := range rs {
		if r.Name == "" {
			continue
		}
		parts = append(parts, r.Name+":"+strconv.Itoa(r.Count))
	}
	return strings.Join(parts, ", ")
}
