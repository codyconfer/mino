package kubectl

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/codyconfer/mino/external/plugins/internal/errx"
	"github.com/codyconfer/mino/plugin"
)

const DefaultSince = time.Hour

type eventList struct {
	Items []eventWire `json:"items"`
}

type eventWire struct {
	Metadata       objectMeta `json:"metadata"`
	Type           string     `json:"type"`
	Reason         string     `json:"reason"`
	Message        string     `json:"message"`
	Count          int        `json:"count"`
	FirstTimestamp time.Time  `json:"firstTimestamp"`
	LastTimestamp  time.Time  `json:"lastTimestamp"`
	EventTime      time.Time  `json:"eventTime"`
	InvolvedObject struct {
		Kind      string `json:"kind"`
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"involvedObject"`
}

func (e eventWire) when() time.Time {
	switch {
	case !e.LastTimestamp.IsZero():
		return e.LastTimestamp
	case !e.EventTime.IsZero():
		return e.EventTime
	case !e.FirstTimestamp.IsZero():
		return e.FirstTimestamp
	default:
		return e.Metadata.CreationTimestamp
	}
}

func fetchEvents(ctx context.Context, r runner, s scope, opts options) plugin.Section {
	raw, err := get(ctx, r, s, "events", "--field-selector", "type=Warning")
	if err != nil {
		return errSection(titleEvents, err)
	}
	sec, err := mapEventsJSON(raw, opts.Since, opts.Limit, opts.now())
	if err != nil {
		return errSection(titleEvents, err)
	}
	return sec
}

func mapEventsJSON(raw []byte, since time.Duration, limit int, now time.Time) (plugin.Section, error) {
	var list eventList
	if err := json.Unmarshal(raw, &list); err != nil {
		return plugin.Section{}, errx.Wrap(err, "kubectl events")
	}
	if since <= 0 {
		since = DefaultSince
	}
	cutoff := now.Add(-since)

	items := make([]plugin.Item, 0, len(list.Items))
	for _, e := range list.Items {
		if e.Type != "" && e.Type != "Warning" {
			continue
		}
		at := e.when()
		if at.Before(cutoff) {
			continue
		}
		obj := e.InvolvedObject
		target := strings.ToLower(obj.Kind) + "/" + obj.Name
		if obj.Namespace != "" {
			target = obj.Namespace + "/" + target
		}
		items = append(items, plugin.Item{
			Kind:      "event",
			Title:     target,
			Subtitle:  e.Reason,
			Body:      eventBody(e),
			Timestamp: at,
			Meta: map[string]string{
				"namespace": obj.Namespace,
				"object":    obj.Kind + "/" + obj.Name,
				"reason":    e.Reason,
				"count":     strconv.Itoa(e.Count),
				"severity":  sevWarning,
			},
		})
	}
	sortItems(items)
	return plugin.Section{
		Signal: SignalName,
		Title:  titleEvents,
		Items:  truncate(items, limit),
		Meta:   map[string]string{"window": since.String()},
	}, nil
}

func eventBody(e eventWire) string {
	msg := strings.Join(strings.Fields(e.Message), " ")
	if e.Count > 1 {
		return fmt.Sprintf("%s (×%d)", msg, e.Count)
	}
	return msg
}

func eventKey(it plugin.Item) string {
	return strings.Join([]string{it.Meta["object"], it.Meta["reason"], it.Meta["count"]}, "\x00")
}
