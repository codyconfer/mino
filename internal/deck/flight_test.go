package deck

import (
	"context"
	"strings"
	"testing"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/mino/internal/signals"
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
	w := make(vkdeck.Work, len(tasks))
	for i, task := range tasks {
		label, run := task.Label, task.Run
		w[i] = vkdeck.Job{
			Label: label,
			Do: func(ctx context.Context) (vkdeck.Content, error) {
				sections := run(ctx)
				body := ""
				if len(sections) > 0 {
					body = sections[0].Title
				}
				return vkdeck.Text(body), nil
			},
		}
	}
	out, err := w.Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out[0].Render(0), "Alpha") {
		t.Fatalf("got %q", out[0].Render(0))
	}
}
