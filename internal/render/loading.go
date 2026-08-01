package render

import (
	"io"
	"time"

	"github.com/codyconfer/viewkit/spin"

	"github.com/codyconfer/mino/internal/render/glyph"
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
}

func StartLoading(opts LoadingOptions) *Loading {
	return spin.Start(spin.Options{
		Writer:      opts.Writer,
		Prefix:      loadingPrefix,
		Message:     opts.Message,
		DoneMessage: opts.DoneMessage,
		DoneGlyph:   glyph.Check(),
		Interval:    opts.Interval,
		Frames:      opts.Frames,
		Force:       opts.Force,
	})
}
