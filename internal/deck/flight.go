package deck

import (
	"context"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/render"
	"github.com/codyconfer/munin/internal/signals"
)

// Task is one munin flight panel. Domain sections are adapted to deck.Content
// at the boundary; deck must not import signals.
type Task struct {
	Label string
	Run   func(ctx context.Context) []signals.Section
}

// RunFlight shows progressive tea UI while tasks run under errgroup (viewkit/deck).
func RunFlight(ctx context.Context, tasks []Task) error {
	vt := make([]vkdeck.Task, len(tasks))
	for i, t := range tasks {
		label := t.Label
		run := t.Run
		vt[i] = vkdeck.Task{
			Label: label,
			Run: func(ctx context.Context) (vkdeck.Content, error) {
				sections := run(ctx)
				return vkdeck.Text(render.RenderTerminalStringTitled(label, sections)), nil
			},
		}
	}
	if err := vkdeck.RunFlight(ctx, vt); err != nil {
		return errs.Wrap(errs.KindInternal, err, "run flight view")
	}
	return nil
}
