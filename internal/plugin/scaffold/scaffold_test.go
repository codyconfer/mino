package scaffold

import (
	"context"
	"testing"

	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/munin/internal/plugin"
)

func TestScaffoldRegister(t *testing.T) {
	Register()
	d, ok := plugin.Lookup(PluginID)
	if !ok || d.Signal != SignalName {
		t.Fatalf("lookup = %+v ok=%v", d, ok)
	}
	if _, ok := glyph.Lookup(GlyphID); !ok {
		t.Fatal("glyph not registered")
	}
	secs, err := Signal{}.Fetch(context.Background())
	if err != nil || len(secs) != 1 {
		t.Fatalf("Fetch = %v err=%v", secs, err)
	}
	if err := plugin.SwitchContext(context.Background(), ContextTool, "demo"); err != nil {
		t.Fatal(err)
	}
}
