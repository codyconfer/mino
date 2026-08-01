package render

import (
	"io"
	"time"

	"github.com/codyconfer/viewkit/spin"
	"github.com/codyconfer/viewkit/ui"
)

const loadingPrefix = "mino ▸"

type Loading = spin.Spinner

type LoadingOptions struct {
	Writer      io.Writer
	Message     string
	DoneMessage string
	Interval    time.Duration
	Frames      []string
	Force       bool
	// UI is the rendering scope. Nil falls back to the process defaults.
	UI *ui.Scope
}

func StartLoading(opts LoadingOptions) *Loading {
	scope := opts.UI
	if scope == nil {
		scope = ui.Default()
	}
	return spin.Start(spin.Options{
		Writer:      opts.Writer,
		Prefix:      loadingPrefix,
		Message:     opts.Message,
		DoneMessage: opts.DoneMessage,
		DoneGlyph:   scope.Glyphs.Check(),
		Theme:       &scope.Theme,
		Interval:    opts.Interval,
		Frames:      opts.Frames,
		Force:       opts.Force,
	})
}
