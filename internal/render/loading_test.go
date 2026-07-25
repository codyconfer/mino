package render

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestLoadingStartStopClearsLine(t *testing.T) {
	var buf bytes.Buffer
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
	var buf bytes.Buffer
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
