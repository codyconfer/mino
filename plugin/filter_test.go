package plugin_test

import (
	"strings"
	"testing"

	"github.com/codyconfer/munin/plugin"
)

func TestRegisterFilterEngineAndRules(t *testing.T) {
	parent := "test.filter.sdk"
	if _, ok := plugin.Lookup(parent); !ok {
		plugin.Register(plugin.Descriptor{
			ID: parent, Kind: plugin.KindSignal, Signal: "testfiltersdk",
			Capabilities: []plugin.Capability{plugin.CapQuery},
		})
	}

	rulesName := "test-sdk-rules"
	if !plugin.HasFilter(rulesName) {
		plugin.RegisterFilter(parent, plugin.NamedFilter{
			Name:  rulesName,
			Rules: []plugin.FilterRule{{Field: "body", Exclude: "noise"}},
		})
	}
	if d, ok := plugin.ByKind(plugin.KindFilter, rulesName); !ok || d.Parent != parent {
		t.Fatalf("rules KindFilter = %+v ok=%v", d, ok)
	}
	if f, ok := plugin.LookupFilter(rulesName); !ok || len(f.Rules) != 1 {
		t.Fatalf("LookupFilter rules = %+v ok=%v", f, ok)
	}

	engName := "test-sdk-engine"
	if !plugin.HasFilter(engName) {
		plugin.RegisterFilterEngine(parent, engName, func(items []plugin.Item) []plugin.Item {
			out := make([]plugin.Item, 0, len(items))
			for _, it := range items {
				if !strings.Contains(strings.ToLower(it.Body), "dropme") {
					out = append(out, it)
				}
			}
			return out
		})
	}
	if !plugin.HasFilterEngine(engName) {
		t.Fatal("expected engine")
	}
	fn, ok := plugin.LookupFilterEngine(engName)
	if !ok {
		t.Fatal("LookupFilterEngine missing")
	}
	got := fn([]plugin.Item{
		{Title: "keep", Body: "ok"},
		{Title: "gone", Body: "DROPME please"},
	})
	if len(got) != 1 || got[0].Title != "keep" {
		t.Fatalf("engine apply = %+v", got)
	}
	if f, ok := plugin.LookupFilter(engName); !ok || f.Name != engName || len(f.Rules) != 0 {
		t.Fatalf("engine stub LookupFilter = %+v ok=%v", f, ok)
	}
}
