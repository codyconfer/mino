package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	vkglyph "github.com/codyconfer/viewkit/glyph"
)

func TestStatusStripLineColorsByTone(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	left, right := StatusStripLine("##", "role", []string{"ctx"}, []vkglyph.StatusContribution{
		{Status: func() (string, vkglyph.Severity) { return "OK", vkglyph.SeverityPositive }},
		{Status: func() (string, vkglyph.Severity) { return "BAD", vkglyph.SeverityNegative }},
	})
	if !strings.Contains(left, "##") || !strings.Contains(left, "role") {
		t.Fatalf("left = %q", left)
	}
	plain := ansi.Strip(right)
	if !strings.Contains(plain, "OK") || !strings.Contains(plain, "BAD") {
		t.Fatalf("right plain = %q (ansi %q)", plain, right)
	}
	if right == plain {
		t.Fatalf("expected ANSI coloring on right strip, got plain %q", right)
	}
}
