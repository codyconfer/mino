package render

import (
	"strings"

	vkglyph "github.com/codyconfer/viewkit/glyph"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/mino/internal/plugin"
	"github.com/codyconfer/mino/internal/render/glyph"
)

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
		rightParts = append(rightParts, theme.StripText(theme.SeverityColor(c.Tone), c.Glyph))
	}
	return strings.Join(strip.Left, " "), strings.Join(rightParts, " ")
}

func ComposeStatusStrip(brand, role, home string, contexts []string) (left, right string) {
	return StatusStripLine(brand, role, contexts, plugin.CollectStatusContributions(home, role))
}
