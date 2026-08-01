package log

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	sconfig "github.com/codyconfer/sisyphus/config"

	"github.com/codyconfer/viewkit/theme"
)

type Level int

const (
	LevelError Level = iota
	LevelWarn
	LevelInfo
	LevelDebug
)

type ColorMode int

const (
	ColorAuto ColorMode = iota
	ColorAlways
	ColorNever
)

var (
	mu    sync.Mutex
	level = LevelWarn
	color = ColorAuto

	console io.Writer = os.Stderr
	file    *os.File

	tags  map[Level]string
	dimSt func(string) string
)

var plainTags = map[Level]string{
	LevelError: "error",
	LevelWarn:  "warn",
	LevelInfo:  "info",
	LevelDebug: "debug",
}

func init() { rebuild() }

func rebuild() {
	plain := func(s string) string { return s }
	if color == ColorNever {
		tags = map[Level]string{
			LevelError: plainTags[LevelError],
			LevelWarn:  plainTags[LevelWarn],
			LevelInfo:  plainTags[LevelInfo],
			LevelDebug: plainTags[LevelDebug],
		}
		dimSt = plain
		return
	}
	th := theme.Cur()
	warn := th.Cant
	if len(th.Series) > 2 {
		warn = th.Series[2]
	}
	tags = map[Level]string{
		LevelError: th.Cant.Bold(true).Render("error"),
		LevelWarn:  warn.Bold(true).Render("warn"),
		LevelInfo:  th.Accent.Bold(true).Render("info"),
		LevelDebug: th.Dim.Render("debug"),
	}
	dimSt = func(s string) string { return th.Dim.Render(s) }
}

func SetOutput(w io.Writer) {
	mu.Lock()
	console = w
	rebuild()
	mu.Unlock()
}

func ClearConsole() {
	mu.Lock()
	console = nil
	rebuild()
	mu.Unlock()
}

func SetFileSink(path string) (io.Closer, error) {
	f, err := sconfig.OpenAppend(path)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	prev := file
	file = f
	mu.Unlock()
	if prev != nil {
		_ = prev.Close()
	}
	return f, nil
}

func CloseFileSink() {
	mu.Lock()
	prev := file
	file = nil
	mu.Unlock()
	if prev != nil {
		_ = prev.Close()
	}
}

func SetLevel(l Level) {
	mu.Lock()
	level = l
	mu.Unlock()
}

func ParseLevel(s string) (Level, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "error":
		return LevelError, true
	case "warn", "warning":
		return LevelWarn, true
	case "info":
		return LevelInfo, true
	case "debug":
		return LevelDebug, true
	}
	return LevelWarn, false
}

func SetVerbose(v bool) {
	if v {
		SetLevel(LevelDebug)
	}
}

func SetColorMode(m ColorMode) {
	mu.Lock()
	color = m
	rebuild()
	mu.Unlock()
}

func ParseColorMode(s string) (ColorMode, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "auto":
		return ColorAuto, true
	case "always", "force", "on", "yes":
		return ColorAlways, true
	case "never", "off", "no", "none":
		return ColorNever, true
	}
	return ColorAuto, false
}

func logf(l Level, format string, args ...any) {
	mu.Lock()
	enabled := l <= level
	tag := tags[l]
	prefix := dimSt("mino ▸")
	cw := console
	fw := file
	mu.Unlock()
	if !enabled {
		return
	}
	msg := fmt.Sprintf(format, args...)
	if cw != nil {
		fmt.Fprintln(cw, prefix+" "+tag+" "+msg)
	}
	if fw != nil {
		fmt.Fprintln(fw, "mino ▸ "+plainTags[l]+" "+msg)
	}
}

func Errorf(format string, args ...any) { logf(LevelError, format, args...) }
func Warnf(format string, args ...any)  { logf(LevelWarn, format, args...) }
func Infof(format string, args ...any)  { logf(LevelInfo, format, args...) }
func Debugf(format string, args ...any) { logf(LevelDebug, format, args...) }
