package log

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
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

	r     *lipgloss.Renderer
	tags  map[Level]string
	dimSt lipgloss.Style
)

func init() { rebuild() }

func rebuild() {
	r = lipgloss.NewRenderer(os.Stderr)
	switch color {
	case ColorNever:
		r.SetColorProfile(termenv.Ascii)
	case ColorAlways:
		if r.ColorProfile() == termenv.Ascii {
			r.SetColorProfile(termenv.TrueColor)
		}
	}
	tags = map[Level]string{
		LevelError: r.NewStyle().Bold(true).Foreground(lipgloss.Color("9")).Render("error"),
		LevelWarn:  r.NewStyle().Bold(true).Foreground(lipgloss.Color("11")).Render("warn"),
		LevelInfo:  r.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Render("info"),
		LevelDebug: r.NewStyle().Faint(true).Render("debug"),
	}
	dimSt = r.NewStyle().Faint(true)
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

func Renderer() *lipgloss.Renderer {
	mu.Lock()
	defer mu.Unlock()
	return r
}

func logf(l Level, format string, args ...any) {
	mu.Lock()
	enabled := l <= level
	tag := tags[l]
	prefix := dimSt.Render("munin ▸")
	mu.Unlock()
	if !enabled {
		return
	}
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(os.Stderr, prefix+" "+tag+" "+msg)
}

func Errorf(format string, args ...any) { logf(LevelError, format, args...) }
func Warnf(format string, args ...any)  { logf(LevelWarn, format, args...) }
func Infof(format string, args ...any)  { logf(LevelInfo, format, args...) }
func Debugf(format string, args ...any) { logf(LevelDebug, format, args...) }
