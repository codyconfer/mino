package render

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/x/term"
	"github.com/codyconfer/viewkit/theme"
)

type Loading struct {
	w        io.Writer
	msg      string
	frames   []string
	every    time.Duration
	stopCh   chan struct{}
	doneCh   chan struct{}
	mu       sync.Mutex
	stopped  bool
	animated bool
}

type LoadingOptions struct {
	Writer   io.Writer
	Message  string
	Interval time.Duration
	Frames   []string
	Force    bool
}

func StartLoading(opts LoadingOptions) *Loading {
	w := opts.Writer
	if w == nil {
		w = os.Stderr
	}
	msg := opts.Message
	if msg == "" {
		msg = "starting…"
	}
	frames := opts.Frames
	if len(frames) == 0 {
		frames = append([]string(nil), spinner.MiniDot.Frames...)
	}
	every := opts.Interval
	if every <= 0 {
		every = spinner.MiniDot.FPS
		if every <= 0 {
			every = time.Second / 12
		}
	}

	l := &Loading{
		w:      w,
		msg:    msg,
		frames: frames,
		every:  every,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
	if !opts.Force && !writerIsTerminal(w) {
		l.stopped = true
		close(l.doneCh)
		return l
	}
	l.animated = true
	l.paint(0)
	go l.loop()
	return l
}

func (l *Loading) Stop() {
	if l == nil {
		return
	}
	l.mu.Lock()
	if l.stopped {
		l.mu.Unlock()
		return
	}
	l.stopped = true
	l.mu.Unlock()

	if !l.animated {
		return
	}
	close(l.stopCh)
	<-l.doneCh
	_, _ = fmt.Fprint(l.w, "\r\033[K")
}

func (l *Loading) loop() {
	defer close(l.doneCh)
	t := time.NewTicker(l.every)
	defer t.Stop()
	i := 0
	for {
		select {
		case <-l.stopCh:
			return
		case <-t.C:
			i++
			l.paint(i)
		}
	}
}

func (l *Loading) paint(i int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.stopped || len(l.frames) == 0 {
		return
	}
	frame := l.frames[i%len(l.frames)]
	prefix := loadingPrefix()
	frameStyled := theme.Cur().Accent.Render(frame)
	msg := theme.Cur().Dim.Render(l.msg)
	_, _ = fmt.Fprintf(l.w, "\r%s %s %s", prefix, frameStyled, msg)
}

func loadingPrefix() string {
	return theme.Cur().Dim.Render("munin ▸")
}

func writerIsTerminal(w io.Writer) bool {
	type fd interface{ Fd() uintptr }
	f, ok := w.(fd)
	if !ok {
		return false
	}
	return term.IsTerminal(f.Fd())
}
