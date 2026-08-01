package log

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/codyconfer/viewkit/theme"
)

func TestSetThemeOverridesDefault(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prevProfile) })

	t.Cleanup(func() {
		mu.Lock()
		injected = nil
		rebuild()
		mu.Unlock()
	})

	SetColorMode(ColorAuto)
	SetLevel(LevelInfo)
	t.Cleanup(func() { SetLevel(LevelWarn) })

	var buf bytes.Buffer
	SetOutput(&buf)
	t.Cleanup(func() { SetOutput(os.Stderr) })

	defTag := theme.Default().Accent.Bold(true).Render("info")
	Infof("x")
	if !strings.Contains(buf.String(), defTag) {
		t.Fatalf("un-injected log should style via theme.Default(), got %q want tag %q", buf.String(), defTag)
	}

	inj := theme.New(theme.Palette{Accent: "#aa00aa", Failure: "#00aaaa", Muted: "#aaaa00"})
	injTag := inj.Accent.Bold(true).Render("info")
	if injTag == defTag {
		t.Fatal("test themes must render distinct tags")
	}
	SetTheme(inj)

	buf.Reset()
	Infof("x")
	if !strings.Contains(buf.String(), injTag) {
		t.Fatalf("injected theme should win, got %q want tag %q", buf.String(), injTag)
	}
	if strings.Contains(buf.String(), defTag) {
		t.Fatalf("injected log still styled via the default, got %q", buf.String())
	}
}
