package render

import (
	"reflect"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/codyconfer/viewkit/theme"
)

func TestMinoPaletteSetsEveryColour(t *testing.T) {
	v := reflect.ValueOf(minoPalette)
	ty := v.Type()
	for i := range ty.NumField() {
		f := ty.Field(i)
		got, ok := v.Field(i).Interface().(lipgloss.Color)
		if !ok {
			t.Fatalf("Palette.%s is %s, expected lipgloss.Color", f.Name, f.Type)
		}
		if string(got) == "" {
			t.Errorf("minoPalette.%s is unset; viewkit renders it as the terminal default", f.Name)
		}
	}
}

func TestMinoThemeSeriesStylesAllHaveAColour(t *testing.T) {
	th := theme.New(minoPalette)
	if len(th.Series) == 0 {
		t.Fatal("theme has no series styles")
	}
	for i, s := range th.Series {
		switch fg := s.GetForeground().(type) {
		case lipgloss.Color:
			if string(fg) == "" {
				t.Errorf("series style %d has an empty colour", i)
			}
		default:
			t.Errorf("series style %d foreground is %T, want a lipgloss.Color", s.GetForeground(), fg)
		}
	}
}
