package render

import (
	"strings"

	vkglyph "github.com/codyconfer/viewkit/glyph"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/render/glyph"
)

// StatusStripLine builds a left/right status strip from brand, role, context
// chips, and plugin status contributions (ADR-12).
// Right chips keep glyph.Severity tone and are colored via theme.
func StatusStripLine(brand, role string, contexts []string, contribs []vkglyph.StatusContribution) (left, right string) {
	if brand == "" {
		brand = glyph.Brand()
	}
	strip := vkglyph.BuildStatusStrip(brand, role, contexts, contribs)
	rightParts := make([]string, 0, len(strip.Right))
	for _, c := range strip.Right {
		if c.Glyph == "" {
			continue
		}
		// glyph.Severity and theme.Severity share iota order
		// (neutral/muted → positive/ok → warning/warn → negative/bad).
		rightParts = append(rightParts, theme.StripText(theme.SeverityColor(theme.Severity(c.Tone)), c.Glyph))
	}
	return strings.Join(strip.Left, " "), strings.Join(rightParts, " ")
}
