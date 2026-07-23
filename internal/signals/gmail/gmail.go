package gmail

import (
	"context"
	"strings"
	"time"

	gmailapi "google.golang.org/api/gmail/v1"

	"github.com/codyconfer/munin/internal/auth"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/signals"
)

type gmailSignal struct {
	query string
	max   int
	auth  auth.GoogleAuth
}

func New(query string, max int, ga auth.GoogleAuth) signals.Signal {
	return &gmailSignal{query: query, max: max, auth: ga}
}

func (g *gmailSignal) Name() string { return "gmail" }

func (g *gmailSignal) Fetch(ctx context.Context) ([]signals.Section, error) {
	query := g.query
	if query == "" {
		query = "is:unread in:inbox"
	}
	max := g.max
	if max <= 0 {
		max = 15
	}

	opt, err := auth.GoogleClientOption(ctx, g.auth, gmailapi.GmailReadonlyScope)
	if err != nil {
		return nil, err
	}
	svc, err := gmailapi.NewService(ctx, opt)
	if err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "gmail: creating service")
	}

	list, err := svc.Users.Messages.List("me").Q(query).MaxResults(int64(max)).Context(ctx).Do()
	if err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "gmail: listing messages")
	}

	items := make([]signals.Item, 0, len(list.Messages))
	for _, m := range list.Messages {
		msg, err := svc.Users.Messages.Get("me", m.Id).
			Format("metadata").
			MetadataHeaders("From", "Subject").
			Context(ctx).
			Do()
		if err != nil {
			return nil, errs.Wrapf(errs.KindSignal, err, "gmail: fetching message %s", m.Id)
		}
		items = append(items, messageToItem(msg))
	}

	return []signals.Section{{
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

func messageToItem(msg *gmailapi.Message) signals.Item {
	from := header(msg, "From")
	subject := header(msg, "Subject")
	if subject == "" {
		subject = "(no subject)"
	}
	return signals.Item{
		Kind:      "email",
		Title:     subject,
		Subtitle:  from,
		Body:      msg.Snippet,
		URL:       "https://mail.google.com/mail/#all/" + msg.Id,
		Timestamp: time.UnixMilli(msg.InternalDate),
		Meta:      map[string]string{"from": from},
	}
}
