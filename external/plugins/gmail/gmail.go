package gmail

import (
	"context"
	"strings"
	"time"

	gmailapi "google.golang.org/api/gmail/v1"

	"github.com/codyconfer/mino/external/plugins/internal/errx"
	"github.com/codyconfer/mino/external/plugins/internal/googleauth"
	"github.com/codyconfer/mino/plugin"
)

type gmailSignal struct {
	query string
	max   int
	auth  googleauth.Auth
}

func New(query string, max int, ga googleauth.Auth) plugin.Query {
	return &gmailSignal{query: query, max: max, auth: ga}
}

func (g *gmailSignal) Name() string { return "gmail" }

func (g *gmailSignal) Fetch(ctx context.Context) ([]plugin.Section, error) {
	query := g.query
	if query == "" {
		query = "is:unread in:inbox"
	}
	max := g.max
	if max <= 0 {
		max = 15
	}

	svc, err := googleauth.Service(ctx, g.auth, "gmail", []string{gmailapi.GmailReadonlyScope}, gmailapi.NewService)
	if err != nil {
		return nil, err
	}

	list, err := svc.Users.Messages.List("me").Q(query).MaxResults(int64(max)).Context(ctx).Do()
	if err != nil {
		return nil, errx.Wrap(err, "gmail: listing messages")
	}

	items := make([]plugin.Item, 0, len(list.Messages))
	for _, m := range list.Messages {
		msg, err := svc.Users.Messages.Get("me", m.Id).
			Format("metadata").
			MetadataHeaders("From", "Subject").
			Context(ctx).
			Do()
		if err != nil {
			return nil, errx.Wrapf(err, "gmail: fetching message %s", m.Id)
		}
		items = append(items, messageToItem(msg))
	}

	return []plugin.Section{{
		Signal: "gmail",
		Title:  "Gmail",
		Items:  items,
	}}, nil
}

func header(msg *gmailapi.Message, name string) string {
	if msg == nil || msg.Payload == nil {
		return ""
	}
	for _, h := range msg.Payload.Headers {
		if strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}

func messageToItem(msg *gmailapi.Message) plugin.Item {
	from := header(msg, "From")
	subject := header(msg, "Subject")
	if subject == "" {
		subject = "(no subject)"
	}
	return plugin.Item{
		Kind:      "email",
		Title:     subject,
		Subtitle:  from,
		Body:      msg.Snippet,
		URL:       "https://mail.google.com/mail/#all/" + msg.Id,
		Timestamp: time.UnixMilli(msg.InternalDate),
		Meta:      map[string]string{"from": from},
	}
}
