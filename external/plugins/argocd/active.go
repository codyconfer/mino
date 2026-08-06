package argocd

import (
	"context"
	"time"

	"github.com/codyconfer/mino/external/plugins/internal/httpx"
	"github.com/codyconfer/mino/external/plugins/internal/stream"
	"github.com/codyconfer/mino/plugin"
)

const (
	seenNamespace   = "argocd:applications"
	cursorNamespace = "argocd"
	cursorKey       = "resource_version"
)

type activeArgo struct {
	client   *Client
	cfg      Config
	interval time.Duration
	state    *stream.State
}

func NewActive(cfg Config, tokens TokenLookup, interval time.Duration, state *stream.State) plugin.Stream {
	return &activeArgo{client: NewClient(cfg, tokens), cfg: cfg, interval: interval, state: state}
}

func (h *activeArgo) Name() string { return SignalName }

func (h *activeArgo) LatencyFloor() time.Duration { return h.interval }

func (h *activeArgo) Stream(ctx context.Context) (<-chan plugin.Event, error) {
	seen := h.state.Seen(seenNamespace)
	cursor := h.state.Cursor(cursorNamespace, cursorKey)
	fails := 0

	step := func(ctx context.Context) ([]plugin.Item, time.Duration, error) {
		list, hdr, err := h.client.Applications(ctx)
		if err != nil {
			fails++
			return nil, max(h.interval, httpx.WithJitter(httpx.Backoff(h.interval, fails))), err
		}
		fails = 0

		if cursor != nil && list.Metadata.ResourceVersion != "" {
			_ = cursor.Save(ctx, list.Metadata.ResourceVersion)
		}

		items := make([]plugin.Item, 0, len(list.Items))
		for _, app := range list.Items {
			if keep(app, h.cfg) {
				items = append(items, applicationToItem(app, h.cfg))
			}
		}
		sortItems(items)

		next := h.interval
		if retry := retryAfterFrom(hdr); retry > 0 {
			next = max(next, httpx.WithJitter(retry))
		}
		return seen.Unseen(ctx, items, appKey), next, nil
	}

	return stream.PollAdaptive(ctx, SignalName, h.interval, step), nil
}

func appKey(it plugin.Item) string {
	return it.Meta["app_namespace"] + "/" + it.Meta["app"] + "|" +
		it.Meta["revision"] + "|" + it.Meta["state"]
}
