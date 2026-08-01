package errs

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/codyconfer/viewkit/glyph"
	"github.com/codyconfer/viewkit/theme"
)

func TestSetThemeOverridesDefault(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prevProfile) })
	t.Cleanup(func() { injected.Store(nil) })

	defMark := theme.Default().Cant.Bold(true).Render(glyph.Cross())
	if got := Render(errors.New("boom")); !strings.HasPrefix(got, defMark+" ") {
		t.Fatalf("un-injected Render should style via theme.Default(), got %q want mark %q", got, defMark)
	}

	inj := theme.New(theme.Palette{Failure: "#00aaaa", Muted: "#aaaa00", Accent: "#aa00aa"})
	injMark := inj.Cant.Bold(true).Render(glyph.Cross())
	if injMark == defMark {
		t.Fatal("test themes must render distinct marks")
	}
	SetTheme(inj)

	if got := Render(errors.New("boom")); !strings.HasPrefix(got, injMark+" ") {
		t.Fatalf("injected theme should win, got %q want mark %q", got, injMark)
	}
}
