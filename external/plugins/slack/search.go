package slack

import (
	"context"
	"strings"

	slackapi "github.com/slack-go/slack"

	"github.com/codyconfer/mino/external/plugins/internal/errx"
	"github.com/codyconfer/mino/plugin"
)

func (s *slackSignal) fetchMentions(ctx context.Context, api *slackapi.Client, who whoami) (plugin.Section, error) {
	if who.userID == "" {
		return plugin.Section{}, errx.New("slack: cannot read mentions without knowing who the token is").
			WithHint("mentions need a user token (xoxp-) that auth.test accepts; run `mino login slack`")
	}
	query := "<@" + who.userID + ">"
	if s.mentionQuery != "" {
		query += " " + s.mentionQuery
	}
	return s.searchSection(ctx, api, who, query, "mentions", "mention")
}

func (s *slackSignal) fetchSearch(ctx context.Context, api *slackapi.Client, who whoami) (plugin.Section, error) {
	return s.searchSection(ctx, api, who, s.search, "search: "+capRunes(s.search, 60), "search")
}

func (s *slackSignal) searchSection(ctx context.Context, api *slackapi.Client, who whoami, query, title, kind string) (plugin.Section, error) {
	res, err := api.SearchMessagesContext(ctx, query, slackapi.SearchParameters{
		Sort:          "timestamp",
		SortDirection: "desc",
		Count:         s.limit,
		Page:          1,
	})
	if err != nil {
		return plugin.Section{}, errx.Wrapf(scopeError(err, "searching messages"), "slack: search %q", capRunes(query, 60))
	}
	if res == nil {
		return plugin.Section{Signal: SignalName, Title: title}, nil
	}

	names := s.resolveUsers(ctx, api, matchUserIDs(res.Matches))
	items := make([]plugin.Item, 0, len(res.Matches))
	for _, m := range res.Matches {
		items = append(items, matchToItem(m, who.host, names, kind))
	}
	return plugin.Section{Signal: SignalName, Title: title, Items: items}, nil
}

func matchUserIDs(matches []slackapi.SearchMessage) []string {
	seen := map[string]bool{}
	for _, m := range matches {
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

func matchToItem(m slackapi.SearchMessage, host string, names map[string]string, kind string) plugin.Item {
	c := itemCtx{
		channelID:   m.Channel.ID,
		channelName: m.Channel.Name,
		host:        host,
		names:       names,
		kind:        kind,
	}
	it := messageToItem(slackapi.Message{Msg: slackapi.Msg{
		Text:      m.Text,
		User:      m.User,
		Username:  m.Username,
		Timestamp: m.Timestamp,
	}}, c)

	if link := strings.TrimSpace(m.Permalink); link != "" {
		it.URL = link
		if ref, ok := parsePermalink(link); ok && ref.ThreadTS != "" {
			it.Meta["thread_ts"] = ref.ThreadTS
		}
	}
	return it
}
