package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	vkglyph "github.com/codyconfer/viewkit/glyph"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/mino/internal/plugin"
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

func TestStatusStripLineUsesSeverityColorNotIotaCast(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	_, right := StatusStripLine("##", "r", nil, []vkglyph.StatusContribution{
		{Status: func() (string, vkglyph.Severity) { return "P", vkglyph.SeverityPositive }},
		{Status: func() (string, vkglyph.Severity) { return "N", vkglyph.SeverityNegative }},
	})
	pos := theme.StripText(theme.SeverityColor(vkglyph.SeverityPositive), "P")
	neg := theme.StripText(theme.SeverityColor(vkglyph.SeverityNegative), "N")
	if !strings.Contains(right, pos) || !strings.Contains(right, neg) {
		t.Fatalf("right=%q missing SeverityColor segments pos=%q neg=%q", right, pos, neg)
	}
	if pos == neg {
		t.Fatal("positive and negative SeverityColor rendered identically")
	}
}

func TestComposeStatusStripIncludesRegisteredContributions(t *testing.T) {
	id := "test.compose.status"
	marker := "COMPOSE-MARK"
	if _, ok := plugin.Lookup(id); !ok {
		plugin.Register(plugin.Descriptor{
			ID:           id,
			Kind:         plugin.KindSignal,
			Signal:       "testcomposestatus",
			Capabilities: []plugin.Capability{plugin.CapQuery},
		})
	}
	plugin.RegisterStatusContribution(id, func(_, _ string) vkglyph.StatusContribution {
		return vkglyph.StatusContribution{
			Status: func() (string, vkglyph.Severity) { return marker, vkglyph.SeverityWarning },
		}
	})

	_, right := ComposeStatusStrip("##", "role", "/tmp", nil)
	plain := ansi.Strip(right)
	if !strings.Contains(plain, marker) {
		t.Fatalf("ComposeStatusStrip right missing %q: %q", marker, plain)
	}
}
