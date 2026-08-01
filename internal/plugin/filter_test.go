package plugin

import (
	"context"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/filter"
	"github.com/codyconfer/mino/internal/signals"
	"github.com/codyconfer/mino/internal/testenv"
)

func TestRegisterFilterLinksKindAndResolve(t *testing.T) {
	id := "test.kinds.filter"
	name := "test-kinds-filter"
	if _, ok := Lookup(id); !ok {
		Register(Descriptor{ID: id, Kind: KindSignal, Signal: "testkindsfilter", Capabilities: []Capability{CapQuery}})
	}
	if !HasFilter(name) {
		RegisterFilter(id, filter.Filter{
			Name:  name,
			Rules: []filter.Rule{{Field: "body", Exclude: "noise"}},
		})
	}
	d, ok := ByKind(KindFilter, name)
	if !ok {
		t.Fatal("expected KindFilter descriptor")
	}
	if d.Parent != id {
		t.Fatalf("parent = %q", d.Parent)
	}
	f, ok := LookupFilter(name)
	if !ok || f.Name != name {
		t.Fatalf("LookupFilter = %+v ok=%v", f, ok)
	}
	if config.ExternalFilter == nil {
		t.Fatal("ExternalFilter not wired")
	}
	got, ok := config.ExternalFilter(name)
	if !ok || got.Name != name {
		t.Fatalf("ExternalFilter = %+v ok=%v", got, ok)
	}
}

func TestRegisterFilterEngineCompileAndApply(t *testing.T) {
	id := "test.kinds.filter.engine"
	name := "test-kinds-engine"
	if _, ok := Lookup(id); !ok {
		Register(Descriptor{ID: id, Kind: KindSignal, Signal: "testkindsengine", Capabilities: []Capability{CapQuery}})
	}
	if !HasFilter(name) {
		RegisterFilterEngine(id, name, func(items []signals.Item) []signals.Item {
			out := make([]signals.Item, 0, len(items))
			for _, it := range items {
				if !strings.EqualFold(it.Kind, "noise") {
					out = append(out, it)
				}
			}
			return out
		})
	}
	if !HasFilterEngine(name) {
		t.Fatal("expected engine registration")
	}
	if filter.ExternalEngine == nil {
		t.Fatal("ExternalEngine not wired")
	}
	c, err := filter.Compile(filter.Filter{Name: name})
	if err != nil {
		t.Fatal(err)
	}
	if !c.IsEngine() {
		t.Fatal("Compile should bind engine")
	}
	got := c.Apply([]signals.Item{
		{Kind: "ok", Title: "keep"},
		{Kind: "noise", Title: "drop"},
	})
	if len(got) != 1 || got[0].Title != "keep" {
		t.Fatalf("Apply = %+v", got)
	}

	f, ok := config.ExternalFilter(name)
	if !ok || f.Name != name {
		t.Fatalf("ExternalFilter stub = %+v ok=%v", f, ok)
	}
}

func TestCompanionEnableInheritsParent(t *testing.T) {
	testenv.Isolate(t)
	id := "test.kinds.enable"
	sig := "testkindsenable"
	if _, ok := Lookup(id); !ok {
		Register(Descriptor{ID: id, Kind: KindSignal, Signal: sig, Capabilities: []Capability{CapQuery, CapAction}})
	}
	if _, ok := LookupAction(sig, "x"); !ok {
		RegisterAction(sig, "x", func(context.Context, map[string]string) error { return nil })
	}
	companion := id + "/action/x"
	if !Enabled(companion) {
		t.Fatal("companion should start enabled with parent")
	}
	t.Cleanup(func() { _ = SetEnabled(id, true) })
	if err := SetEnabled(id, false); err != nil {
		t.Fatal(err)
	}
	if Enabled(companion) {
		t.Fatal("companion should disable with parent")
	}
	if err := SetEnabled(companion, true); err == nil {
		t.Fatal("expected error toggling companion directly")
	}
}
