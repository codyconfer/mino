package plugin

import (
	"testing"

	"github.com/codyconfer/viewkit/glyph"
)

func TestCollectStatusContributionsRespectsEnable(t *testing.T) {
	home := t.TempDir()
	id := "test.status.strip"
	if _, ok := Lookup(id); !ok {
		Register(Descriptor{
			ID:           id,
			Kind:         KindSignal,
			Signal:       "teststatusstrip",
			Capabilities: []Capability{CapQuery},
		})
	}
	marker := "STRIP-MARK"
	RegisterStatusContribution(id, func(_, _ string) glyph.StatusContribution {
		return glyph.StatusContribution{
			Info:   func() string { return "teststatus" },
			Status: func() (string, glyph.Severity) { return marker, glyph.SeverityPositive },
		}
	})

	enableMu.Lock()
	prevDisabled, prevLoaded := disabled, loaded
	disabled = map[string]bool{}
	loaded = true
	enableMu.Unlock()
	t.Cleanup(func() {
		enableMu.Lock()
		disabled, loaded = prevDisabled, prevLoaded
		enableMu.Unlock()
	})

	got := CollectStatusContributions(home, "")
	if !contribHasGlyph(got, marker) {
		t.Fatalf("expected enabled contribution %q in %#v", marker, glyphsOf(got))
	}

	enableMu.Lock()
	disabled[id] = true
	enableMu.Unlock()

	got = CollectStatusContributions(home, "")
	if contribHasGlyph(got, marker) {
		t.Fatalf("disabled contribution still present: %#v", glyphsOf(got))
	}
}

func contribHasGlyph(contribs []glyph.StatusContribution, want string) bool {
	for _, c := range contribs {
		if c.Status == nil {
			continue
		}
		if g, _ := c.Status(); g == want {
			return true
		}
	}
	return false
}

func glyphsOf(contribs []glyph.StatusContribution) []string {
	var out []string
	for _, c := range contribs {
		if c.Status == nil {
			continue
		}
		g, _ := c.Status()
		out = append(out, g)
	}
	return out
}
