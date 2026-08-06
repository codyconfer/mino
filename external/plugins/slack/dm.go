package slack

import (
	"context"
	"sort"

	slackapi "github.com/slack-go/slack"

	"github.com/codyconfer/mino/external/plugins/internal/errx"
	"github.com/codyconfer/mino/plugin"
)

const dmHistoryLimit = 10

func (s *slackSignal) fetchDMs(ctx context.Context, api *slackapi.Client, who whoami) (plugin.Section, error) {
	convos, err := s.listDMs(ctx, api)
	if err != nil {
		return plugin.Section{}, err
	}
	if len(convos) > s.dms {
		convos = convos[:s.dms]
	}

	peerIDs := make([]string, 0, len(convos))
	for _, c := range convos {
		if c.User != "" {
			peerIDs = append(peerIDs, c.User)
		}
	}
	names := s.resolveUsers(ctx, api, peerIDs)

	limit := s.limit
	if limit > dmHistoryLimit {
		limit = dmHistoryLimit
	}

	var items []plugin.Item
	for _, convo := range convos {
		resp, histErr := api.GetConversationHistoryContext(ctx, &slackapi.GetConversationHistoryParameters{
			ChannelID: convo.ID,
			Limit:     limit,
		})
		if histErr != nil {
			continue
		}
		peer := names[convo.User]
		if peer == "" {
			peer = convo.User
		}
		if peer == "" {
			peer = convo.Name
		}
		msgNames := s.resolveUsers(ctx, api, messageUserIDs(resp.Messages))
		for id, name := range names {
			if _, ok := msgNames[id]; !ok {
				msgNames[id] = name
			}
		}
		c := itemCtx{
			channelID:   convo.ID,
			channelName: peer,
			host:        who.host,
			names:       msgNames,
			kind:        "dm",
		}
		for _, msg := range resp.Messages {
			items = append(items, messageToItem(msg, c))
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Timestamp.After(items[j].Timestamp)
	})
	return plugin.Section{Signal: SignalName, Title: "dms", Items: items}, nil
}

func (s *slackSignal) listDMs(ctx context.Context, api *slackapi.Client) ([]slackapi.Channel, error) {
	var out []slackapi.Channel
	cursor := ""
	for page := 0; page < maxListPages; page++ {
		convos, next, err := api.GetConversationsContext(ctx, &slackapi.GetConversationsParameters{
			Types:           []string{"im", "mpim"},
			Cursor:          cursor,
			Limit:           listPageSize,
			ExcludeArchived: true,
		})
		if err != nil {
			return nil, errx.Wrap(scopeError(err, "listing direct messages"), "slack: listing direct messages")
		}
		out = append(out, convos...)
		if len(out) >= s.dms || next == "" || next == cursor {
			break
		}
		cursor = next
	}
	return out, nil
}
