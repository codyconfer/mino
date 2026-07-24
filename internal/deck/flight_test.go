package deck

import (
	"context"
	"strings"
	"testing"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/munin/internal/signals"
)

func TestFlightTaskAdaptsSectionsToContent(t *testing.T) {
	tasks := []Task{{
		Label: "alpha",
		Run: func(context.Context) []signals.Section {
			return []signals.Section{{
				Signal: "alpha",
				Title:  "Alpha",
				Items:  []signals.Item{{Title: "one"}},
			}}
		},
	}}
	vt := make([]vkdeck.Task, len(tasks))
	for i, task := range tasks {
		label, run := task.Label, task.Run
		vt[i] = vkdeck.Task{
			Label: label,
			Run: func(ctx context.Context) (vkdeck.Content, error) {
				sections := run(ctx)
				// mirror RunFlight boundary
				body := ""
				if len(sections) > 0 {
					body = sections[0].Title
				}
				return vkdeck.Text(body), nil
			},
		}
	}
	out, err := vkdeck.Execute(context.Background(), vt)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out[0].Render(0), "Alpha") {
		t.Fatalf("got %q", out[0].Render(0))
	}
}
