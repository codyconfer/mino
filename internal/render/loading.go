package render

import (
	"io"
	"sync"
	"time"

	"github.com/codyconfer/viewkit/spin"
	"github.com/codyconfer/viewkit/ui"

	"github.com/codyconfer/mino/internal/console"
)

const loadingPrefix = "mino ▸"

type Loading struct {
	spinner   *spin.Spinner
	stopTitle func()
	titleOnce sync.Once
}

func (l *Loading) Done() {
	if l == nil {
		return
	}
	l.spinner.Done()
	l.titleOnce.Do(l.stopTitle)
}

func (l *Loading) Stop() {
	if l == nil {
		return
	}
	l.spinner.Stop()
	l.titleOnce.Do(l.stopTitle)
}

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
	spinner := spin.Start(spin.Options{
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
	return &Loading{spinner: spinner, stopTitle: console.StartLoading()}
}
