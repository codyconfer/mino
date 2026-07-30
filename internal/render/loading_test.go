package render

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *lockedBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Len()
}

func TestLoadingStartStopClearsLine(t *testing.T) {
	var buf lockedBuffer
	l := StartLoading(LoadingOptions{
		Writer:   &buf,
		Message:  "starting…",
		Force:    true,
		Interval: 5 * time.Millisecond,
		Frames:   []string{"⠁", "⠂"},
	})

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), "starting…") || strings.Contains(stripANSI(buf.String()), "starting…") {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	got := stripANSI(buf.String())
	if !strings.Contains(got, "munin ▸") {
		t.Fatalf("expected munin prefix, got %q", got)
	}
	if !strings.Contains(got, "starting…") {
		t.Fatalf("expected message, got %q", got)
	}
	if !strings.Contains(got, "⠁") && !strings.Contains(got, "⠂") {
		t.Fatalf("expected a spinner frame, got %q", got)
	}

	l.Stop()
	l.Stop()

	out := buf.String()
	if !strings.Contains(out, "\r\033[K") {
		t.Fatalf("expected clear sequence after Stop, got %q", out)
	}
}

func TestLoadingNonTTYNoOp(t *testing.T) {
	var buf lockedBuffer
	l := StartLoading(LoadingOptions{
		Writer:  &buf,
		Message: "starting…",
	})
	time.Sleep(20 * time.Millisecond)
	l.Stop()
	if buf.Len() != 0 {
		t.Fatalf("non-TTY should not write, got %q", buf.String())
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			i++
			for i < len(s) && ((s[i] >= '0' && s[i] <= '9') || s[i] == '[' || s[i] == ';' || s[i] == '?') {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func TestLoadingDoneSettlesIntoACheckmark(t *testing.T) {
	var buf lockedBuffer
	l := StartLoading(LoadingOptions{
		Writer:      &buf,
		Message:     "starting…",
		DoneMessage: "ready",
		Force:       true,
		Interval:    5 * time.Millisecond,
		Frames:      []string{"⠁", "⠂"},
	})

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) && !strings.Contains(buf.String(), "⠂") {
		time.Sleep(5 * time.Millisecond)
	}
	l.Done()

	out := buf.String()
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("settled line should end with a newline so later output does not overwrite it: %q", out)
	}
	tailAt := strings.LastIndex(out, "\033[K")
	if tailAt < 0 {
		t.Fatalf("no line clear before the settled line: %q", out)
	}
	tail := out[tailAt:]
	if !strings.Contains(tail, "ready") {
		t.Errorf("settled line missing the done message: %q", tail)
	}
	for _, frame := range []string{"⠁", "⠂"} {
		if strings.Contains(tail, frame) {
			t.Errorf("settled line still shows spinner frame %q: %q", frame, tail)
		}
	}
	if strings.Contains(tail, "starting…") {
		t.Errorf("settled line should replace the in-progress message: %q", tail)
	}
}

func TestLoadingDoneOnNonTTYStaysSilent(t *testing.T) {
	var buf lockedBuffer
	l := StartLoading(LoadingOptions{Writer: &buf, Message: "starting…", DoneMessage: "ready"})
	l.Done()
	if buf.Len() != 0 {
		t.Errorf("non-TTY Done should print nothing, got %q", buf.String())
	}
}
