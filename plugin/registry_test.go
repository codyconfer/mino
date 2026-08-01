package plugin_test

import (
	"testing"

	"github.com/codyconfer/mino/plugin"
)

func TestIsInternal(t *testing.T) {
	if !plugin.IsInternal("mino.demo") {
		t.Fatal("expected mino.demo internal")
	}
	if plugin.IsInternal("acme.widgets") {
		t.Fatal("expected acme.widgets external")
	}
	if plugin.IsInternal("minox.fake") {
		t.Fatal("prefix must be mino.")
	}
}

func TestPrimariesInternalFirst(t *testing.T) {
	type fix struct {
		id, signal string
	}
	for _, f := range []fix{
		{"zzz.overlay.sort", "testsortzzz"},
		{"aaa.overlay.sort", "testsortaaa"},
		{"mino.test.sort.b", "testsortrab"},
		{"mino.test.sort.a", "testsortraa"},
	} {
		if _, ok := plugin.Lookup(f.id); ok {
			continue
		}
		plugin.Register(plugin.Descriptor{
			ID:           f.id,
			Kind:         plugin.KindSignal,
			Signal:       f.signal,
			Capabilities: []plugin.Capability{plugin.CapQuery},
		})
	}

	primaries := plugin.Primaries()
	idx := map[string]int{}
	seenExternal := false
	var lastInternal, lastExternal string
	for i, d := range primaries {
		idx[d.ID] = i
		if plugin.IsInternal(d.ID) {
			if seenExternal {
				t.Fatalf("internal %q after external plugin in Primaries", d.ID)
			}
			if lastInternal != "" && d.ID < lastInternal {
				t.Fatalf("internal ids not alpha: %q before %q", lastInternal, d.ID)
			}
			lastInternal = d.ID
			continue
		}
		seenExternal = true
		if lastExternal != "" && d.ID < lastExternal {
			t.Fatalf("external ids not alpha: %q before %q", lastExternal, d.ID)
		}
		lastExternal = d.ID
	}

	for _, id := range []string{
		"mino.test.sort.a", "mino.test.sort.b",
		"aaa.overlay.sort", "zzz.overlay.sort",
	} {
		if _, ok := idx[id]; !ok {
			t.Fatalf("missing primary %q", id)
		}
	}
	if idx["mino.test.sort.a"] > idx["mino.test.sort.b"] {
		t.Fatalf("internal not alpha: a=%d b=%d", idx["mino.test.sort.a"], idx["mino.test.sort.b"])
	}
	if idx["aaa.overlay.sort"] > idx["zzz.overlay.sort"] {
		t.Fatalf("external not alpha: aaa=%d zzz=%d", idx["aaa.overlay.sort"], idx["zzz.overlay.sort"])
	}
	if idx["mino.test.sort.b"] > idx["aaa.overlay.sort"] {
		t.Fatalf("internal after external: mino.b=%d aaa=%d", idx["mino.test.sort.b"], idx["aaa.overlay.sort"])
	}
}
