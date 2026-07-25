package render

import (
	"testing"

	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/render/glyph"
)

func TestKindStyleNegativeUsesErrorTone(t *testing.T) {
	if got := glyph.Classify("error"); got != glyph.KindNegative {
		t.Fatalf("Classify(error) = %v, want KindNegative", got)
	}
	th := theme.Cur()
	neg := kindStyle(th, "error").Render("x")
	dim := th.Dim.Render("x")
	cant := th.Cant.Render("x")
	if neg == dim && neg != cant {
		t.Fatal("SeverityNegative mapped to Dim")
	}
	if neg != cant {
		t.Fatalf("negative render = %q, want Cant %q", neg, cant)
	}
}
