package demo

import (
	"context"
	"fmt"
	"time"

	"github.com/codyconfer/munin/plugin"
)

type Signal struct {
	Base time.Time
}

func (s Signal) Name() string { return "demo" }

func (s Signal) LatencyFloor() time.Duration { return 0 }

func (s Signal) Stream(ctx context.Context) (<-chan plugin.Event, error) {
	out := make(chan plugin.Event)
	go func() {
		defer close(out)
		t := time.NewTicker(3 * time.Second)
		defer t.Stop()
		n := 0
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-t.C:
				n++
				author := "alice"
				if n%2 == 0 {
					author = "deploy-bot"
				}
				ev := plugin.Event{Source: "demo", At: now, Section: plugin.Section{
					Signal: "demo", Title: "Demo",
					Items: []plugin.Item{{
						Kind:      "message",
						Title:     fmt.Sprintf("demo event #%d", n),
						Subtitle:  "#eng-standup",
						Timestamp: now,
						Meta:      map[string]string{"author": author},
					}},
				}}
				select {
				case out <- ev:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

func (s Signal) Fetch(_ context.Context) ([]plugin.Section, error) {
	base := s.Base
	if base.IsZero() {
		base = time.Now()
	}
	return []plugin.Section{{
		Signal: "demo",
		Title:  "Demo Items",
		Items: []plugin.Item{
			{
				Kind: "message", Title: "Deploy finished for api-gateway",
				Subtitle: "#eng-standup", Body: "deploy succeeded in 4m",
				URL: "https://example.com/1", Timestamp: base.Add(-2 * time.Hour),
				Meta: map[string]string{"author": "alice"},
			},
			{
				Kind: "message", Title: "CI passed on munin#42",
				Subtitle: "#eng-standup", Body: "all checks green",
				URL: "https://example.com/2", Timestamp: base.Add(-30 * time.Minute),
				Meta: map[string]string{"author": "deploy-bot"},
			},
		},
	}}, nil
}
