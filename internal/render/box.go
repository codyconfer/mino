package render

import (
	"github.com/codyconfer/viewkit/layout"
)

func TitledBox(f layout.Frame, focused bool, title string, lines ...string) string {
	return TitledBoxIcon(f, focused, "", title, lines...)
}

func TitledBoxIcon(f layout.Frame, focused bool, icon, title string, lines ...string) string {
	f.Focused = focused
	return f.TitledBoxIcon(icon, title, lines...)
}
