package deck

import (
	"context"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/render"
	"github.com/codyconfer/mino/internal/signals"
)

type Task struct {
	Label string
	Run   func(ctx context.Context) []signals.Section
}

func RunFlight(ctx context.Context, tasks []Task) error {
	w := make(vkdeck.Work, len(tasks))
	for i, t := range tasks {
		label := t.Label
		run := t.Run
		w[i] = vkdeck.Job{
			Label: label,
			Run: func(ctx context.Context) (vkdeck.Content, error) {
				sections := run(ctx)
				return vkdeck.Text(render.RenderTerminalStringTitled(label, sections)), nil
			},
		}
	}
	if err := w.RunInteractive(ctx); err != nil {
		return errs.Wrap(errs.KindInternal, err, "run flight view")
	}
	return nil
}
