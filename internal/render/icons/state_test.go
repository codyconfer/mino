package icons

import (
	"runtime"
	"testing"

	"github.com/codyconfer/sisyphus/tray"
)

func TestRegisterEmbedsStateIcons(t *testing.T) {
	Register("dark")
	for _, s := range tray.States() {
		a, ok := tray.IconFor(s)
		if !ok || len(a.Bytes) == 0 {
			t.Fatalf("missing embedded icon for state %s", s)
		}
		if runtime.GOOS == "windows" {
			if a.MIME != "image/x-icon" || !isICO(a.Bytes) {
				t.Fatalf("state %s: want Windows ICO, mime=%q", s, a.MIME)
			}
			continue
		}
		if a.MIME != "image/png" {
			t.Fatalf("state %s: MIME = %q, want image/png", s, a.MIME)
		}
		if !isPNG(a.Bytes) {
			t.Fatalf("state %s: not a PNG", s)
		}
	}
}
