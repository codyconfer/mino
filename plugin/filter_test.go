package plugin_test

import (
	"strings"
	"testing"

	"github.com/codyconfer/mino/plugin"
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

func TestRegisterFilterAliasesAndKeywords(t *testing.T) {
	parent := "test.filter.aliases"
	if _, ok := plugin.Lookup(parent); !ok {
		plugin.Register(plugin.Descriptor{
			ID: parent, Kind: plugin.KindSignal, Signal: "testfilteraliases",
			Capabilities: []plugin.Capability{plugin.CapQuery},
		})
	}
	name := "test-sdk-aliases"
	if !plugin.HasFilter(name) {
		plugin.RegisterFilter(parent, plugin.NamedFilter{
			Name:    name,
			Aliases: map[string]string{"REPOS_ALIAS": "repo:org/a"},
			Keywords: map[string]string{
				"TEAM": "datasources",
			},
		})
	}
	f, ok := plugin.LookupFilter(name)
	if !ok || f.Aliases["REPOS_ALIAS"] != "repo:org/a" || f.Keywords["TEAM"] != "datasources" {
		t.Fatalf("LookupFilter = %+v ok=%v", f, ok)
	}

	kwName := "test-sdk-computed"
	if !plugin.HasFilterKeywords(kwName) {
		plugin.RegisterFilterKeywords(parent, kwName, func() map[string]string {
			return map[string]string{"WINDOW": "created:>=2026-01-01"}
		})
	}
	m, ok := plugin.LookupFilterKeywords(kwName)
	if !ok || m["WINDOW"] != "created:>=2026-01-01" {
		t.Fatalf("LookupFilterKeywords = %#v ok=%v", m, ok)
	}
}
