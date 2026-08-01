package console

import (
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

var (
	stateMu      sync.Mutex
	stateTitle   = "mino"
	stateWriter  io.Writer
	stateLoading *loadingState
)

var loadingFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const loadingInterval = 80 * time.Millisecond

type loadingState struct {
	refs int
	stop chan struct{}
}

func Writer() io.Writer {
	if term.IsTerminal(os.Stdout.Fd()) {
		return os.Stdout
	}
	if term.IsTerminal(os.Stderr.Fd()) {
		return os.Stderr
	}
	return nil
}

func Title(parts ...string) string {
	out := []string{"mino"}
	for _, part := range parts {
		part = clean(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, " · ")
}

func Set(w io.Writer, title string) error {
	if w == nil {
		return nil
	}
	title = clean(title)
	stateMu.Lock()
	stateTitle = title
	stateWriter = w
	loading := stateLoading != nil
	stateMu.Unlock()
	seq := ""
	if !loading {
		seq = ansi.SetIconNameWindowTitle(title)
	}
	if cwd, err := os.Getwd(); err == nil {
		host, _ := os.Hostname()
		if host == "" {
			host = "localhost"
		}
		seq += ansi.NotifyWorkingDirectory(host, cwd)
	}
	_, err := io.WriteString(w, seq)
	return err
}

func Remember(title string) {
	stateMu.Lock()
	stateTitle = clean(title)
	stateMu.Unlock()
}

func StartLoading() func() {
	return startLoading(Writer())
}

func startLoading(w io.Writer) func() {
	if w == nil {
		return func() {}
	}
	stateMu.Lock()
	if stateWriter == nil {
		stateWriter = w
	}
	if stateLoading != nil {
		stateLoading.refs++
		stateMu.Unlock()
		return loadingDone
	}
	state := &loadingState{refs: 1, stop: make(chan struct{})}
	stateLoading = state
	title, writer := stateTitle, stateWriter
	stateMu.Unlock()
	writeLoadingTitle(writer, title, 0)
	go animateLoading(state)
	return loadingDone
}

func animateLoading(state *loadingState) {
	ticker := time.NewTicker(loadingInterval)
	defer ticker.Stop()
	frame := 1
	for {
		select {
		case <-state.stop:
			return
		case <-ticker.C:
			stateMu.Lock()
			if stateLoading != state {
				stateMu.Unlock()
				return
			}
			title, writer := stateTitle, stateWriter
			stateMu.Unlock()
			writeLoadingTitle(writer, title, frame)
			frame++
		}
	}
}

func loadingDone() {
	stateMu.Lock()
	if stateLoading == nil {
		stateMu.Unlock()
		return
	}
	stateLoading.refs--
	if stateLoading.refs > 0 {
		stateMu.Unlock()
		return
	}
	state := stateLoading
	stateLoading = nil
	title, writer := stateTitle, stateWriter
	close(state.stop)
	stateMu.Unlock()
	_, _ = io.WriteString(writer, ansi.SetIconNameWindowTitle(title))
}

func writeLoadingTitle(w io.Writer, title string, frame int) {
	_, _ = io.WriteString(w, ansi.SetIconNameWindowTitle(loadingFrames[frame%len(loadingFrames)]+" "+title))
}

func clean(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}
