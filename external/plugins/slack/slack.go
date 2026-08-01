package slack

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	slackapi "github.com/slack-go/slack"

	"github.com/codyconfer/mino/external/plugins/internal/errx"
	"github.com/codyconfer/mino/plugin"
)

const (
	defaultLimit  = 50
	listPageSize  = 200
	maxListPages  = 50
	chanCacheTTL  = 10 * time.Minute
	chanCacheKeep = 256
)

type resolved struct {
	id   string
	name string
	at   time.Time
}

var chanCache struct {
	mu sync.Mutex
	m  map[string]resolved
}

func cacheKey(token, want string) string { return token + "\x00" + want }

func cachedChannel(token, want string) (resolved, bool) {
	chanCache.mu.Lock()
	defer chanCache.mu.Unlock()
	r, ok := chanCache.m[cacheKey(token, want)]
	if !ok || time.Since(r.at) > chanCacheTTL {
		return resolved{}, false
	}
	return r, true
}

func cacheChannel(token, want, id, name string) {
	chanCache.mu.Lock()
	defer chanCache.mu.Unlock()
	if chanCache.m == nil || len(chanCache.m) > chanCacheKeep {
		chanCache.m = map[string]resolved{}
	}
	chanCache.m[cacheKey(token, want)] = resolved{id: id, name: name, at: time.Now()}
}

func resetChannelCache() {
	chanCache.mu.Lock()
	chanCache.m = nil
	chanCache.mu.Unlock()
}

type slackSignal struct {
	token   string
	channel string
	limit   int
	apiURL  string
}

func New(token, channel string, limit int) plugin.Query {
	if limit <= 0 {
		limit = defaultLimit
	}
	return &slackSignal{token: token, channel: channel, limit: limit}
}

func (s *slackSignal) Name() string { return "slack" }

func (s *slackSignal) client() *slackapi.Client {
	if s.apiURL == "" {
		return slackapi.New(s.token)
	}
	return slackapi.New(s.token, slackapi.OptionAPIURL(s.apiURL))
}

func (s *slackSignal) Fetch(ctx context.Context) ([]plugin.Section, error) {
	api := s.client()

	id, name, err := s.resolveChannel(ctx, api)
	if err != nil {
		return nil, err
	}

	resp, err := api.GetConversationHistoryContext(ctx, &slackapi.GetConversationHistoryParameters{
		ChannelID: id,
		Limit:     s.limit,
	})
	if err != nil {
		return nil, errx.Wrapf(err, "slack: fetching history for %s", id)
	}

	title := "#" + name
	if name == "" {
		title = id
	}

	items := make([]plugin.Item, 0, len(resp.Messages))
	for _, msg := range resp.Messages {
		items = append(items, messageToItem(msg, name))
	}

	return []plugin.Section{{
		Signal: "slack",
		Title:  title,
		Items:  items,
	}}, nil
}

func (s *slackSignal) resolveChannel(ctx context.Context, api *slackapi.Client) (id, name string, err error) {
	if isChannelID(s.channel) {
		return s.channel, "", nil
	}

	want := strings.TrimPrefix(s.channel, "#")
	if hit, ok := cachedChannel(s.token, want); ok {
		return hit.id, hit.name, nil
	}

	cursor := ""
	giveUp := ""
	for page := 0; ; page++ {
		if page >= maxListPages {
			giveUp = fmt.Sprintf("stopped after %d pages", maxListPages)
			break
		}
		channels, next, listErr := api.GetConversationsContext(ctx, &slackapi.GetConversationsParameters{
			Types:  []string{"public_channel", "private_channel"},
			Cursor: cursor,
			Limit:  listPageSize,
		})
		if listErr != nil {
			return "", "", errx.Wrapf(listErr, "slack: listing conversations (page %d)", page+1).
				WithHint("retry in a moment, or set the channel by ID (C.../G.../D...) to skip the channel walk")
		}
		for _, ch := range channels {
			if ch.Name == want {
				cacheChannel(s.token, want, ch.ID, ch.Name)
				return ch.ID, ch.Name, nil
			}
		}
		if next == "" {
			break
		}
		if next == cursor {
			giveUp = "conversations.list returned the same pagination cursor twice"
			break
		}
		cursor = next
	}

	if giveUp != "" {
		return "", "", errx.Newf("slack: gave up resolving channel %q: %s", s.channel, giveUp).
			WithHint("use the channel ID (C.../G.../D...) instead of its name to skip the channel walk")
	}
	return "", "", errx.Newf("slack: channel %q not found", s.channel).
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

func messageToItem(msg slackapi.Message, channelName string) plugin.Item {
	title := firstLine(msg.Text)
	if title == "" {
		title = "(no text)"
	}
	title = capRunes(title, 120)

	subtitle := ""
	if channelName != "" {
		subtitle = "#" + channelName
	}

	return plugin.Item{
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
