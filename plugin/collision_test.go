package plugin_test

import (
	"context"
	"testing"

	"github.com/codyconfer/munin/plugin"
)

func TestDuplicateSignalRefSkipsContributionAndKeepsRegistryUsable(t *testing.T) {
	const good = "collide.good"
	const bad = "collide.bad"
	const signal = "collidesignal"

	plugin.Register(plugin.Descriptor{
		ID:           good,
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapQuery},
	})
	plugin.Register(plugin.Descriptor{
		ID:     bad,
		Kind:   plugin.KindSignal,
		Signal: signal,
	})

	d, ok := plugin.BySignal(signal)
	if !ok || d.ID != good {
		t.Fatalf("BySignal(%q) = %+v, %v; want owner %q", signal, d, ok, good)
	}
	if _, ok := plugin.Lookup(bad); ok {
		t.Fatalf("colliding contribution %q must be skipped, not registered", bad)
	}

	if _, ok := plugin.Lookup(good); !ok {
		t.Fatal("registry lost the incumbent after a collision")
	}
	if len(plugin.Primaries()) == 0 {
		t.Fatal("Primaries empty after a collision (plugins list would be broken)")
	}
	if len(plugin.All()) == 0 {
		t.Fatal("All empty after a collision")
	}
}

func TestDuplicateIDSkipsContribution(t *testing.T) {
	const id = "collide.dupid"

	plugin.Register(plugin.Descriptor{
		ID:     id,
		Kind:   plugin.KindSignal,
		Signal: "collidedupidfirst",
	})
	plugin.Register(plugin.Descriptor{
		ID:     id,
		Kind:   plugin.KindSignal,
		Signal: "collidedupidsecond",
	})

	if _, ok := plugin.BySignal("collidedupidsecond"); ok {
		t.Fatal("second registration of a duplicate id must be skipped")
	}
	if _, ok := plugin.BySignal("collidedupidfirst"); !ok {
		t.Fatal("first registration lost")
	}
}

func TestInvalidDescriptorsAreSkippedNotFatal(t *testing.T) {
	plugin.Register(plugin.Descriptor{ID: "", Kind: plugin.KindSignal, Signal: "collideempty"})
	plugin.Register(plugin.Descriptor{ID: "collide.badkind", Kind: plugin.Kind("nope"), Ref: "x"})
	plugin.Register(plugin.Descriptor{ID: "collide.nosignal", Kind: plugin.KindSignal})
	plugin.Register(plugin.Descriptor{ID: "collide.noref", Kind: plugin.KindFilter})

	for _, id := range []string{"collide.badkind", "collide.nosignal", "collide.noref"} {
		if _, ok := plugin.Lookup(id); ok {
			t.Fatalf("invalid descriptor %q must not be registered", id)
		}
	}
}

func TestDuplicateBuildersSkipped(t *testing.T) {
	const signal = "collidebuilders"

	plugin.RegisterBuilders(signal, plugin.Builders{
		Query: func(plugin.BuildContext) (plugin.Query, error) {
			return testQuery{name: "first"}, nil
		},
	})
	plugin.RegisterBuilders(signal, plugin.Builders{
		Query: func(plugin.BuildContext) (plugin.Query, error) {
			return testQuery{name: "second"}, nil
		},
	})

	q, err := plugin.BuildQuery(signal, testBuildCtx{})
	if err != nil {
		t.Fatal(err)
	}
	if q.Name() != "first" {
		t.Fatalf("builder = %q, want the incumbent %q", q.Name(), "first")
	}
}

func TestDuplicateActionSkipped(t *testing.T) {
	const signal = "collideaction"

	plugin.Register(plugin.Descriptor{
		ID:           "collide.action",
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapAction},
	})
	plugin.RegisterAction(signal, "go", func(context.Context, map[string]string) error { return nil })
	plugin.RegisterAction(signal, "go", func(context.Context, map[string]string) error {
		t.Error("duplicate action ran")
		return nil
	})

	if err := plugin.RunAction(context.Background(), signal, "go", nil); err != nil {
		t.Fatalf("RunAction: %v", err)
	}
}

func TestDuplicateFilterSkipped(t *testing.T) {
	const name = "collidefilter"

	plugin.Register(plugin.Descriptor{ID: "collide.filterowner", Kind: plugin.KindSignal, Signal: "collidefilterowner"})
	plugin.RegisterFilter("collide.filterowner", plugin.NamedFilter{
		Name:  name,
		Rules: []plugin.FilterRule{{Field: "title", Include: "keep"}},
	})
	plugin.RegisterFilter("collide.filterowner", plugin.NamedFilter{
		Name:  name,
		Rules: []plugin.FilterRule{{Field: "title", Include: "clobbered"}},
	})

	f, ok := plugin.LookupFilter(name)
	if !ok {
		t.Fatal("filter lost")
	}
	if len(f.Rules) != 1 || f.Rules[0].Include != "keep" {
		t.Fatalf("filter = %+v, want the incumbent rule", f)
	}
}

func TestBadFilterRegexSkipped(t *testing.T) {
	plugin.Register(plugin.Descriptor{ID: "collide.badregex", Kind: plugin.KindSignal, Signal: "collidebadregex"})
	plugin.RegisterFilter("collide.badregex", plugin.NamedFilter{
		Name:  "collidebadregexfilter",
		Rules: []plugin.FilterRule{{Field: "title", Include: "("}},
	})
	if plugin.HasFilter("collidebadregexfilter") {
		t.Fatal("filter with an uncompilable regex must be skipped")
	}
}

func TestDuplicateFilterEngineSkipped(t *testing.T) {
	const name = "collideengine"

	plugin.Register(plugin.Descriptor{ID: "collide.engineowner", Kind: plugin.KindSignal, Signal: "collideengineowner"})
	plugin.RegisterFilterEngine("collide.engineowner", name, func(items []plugin.Item) []plugin.Item {
		return []plugin.Item{{Title: "first"}}
	})
	plugin.RegisterFilterEngine("collide.engineowner", name, func(items []plugin.Item) []plugin.Item {
		return []plugin.Item{{Title: "second"}}
	})

	fn, ok := plugin.LookupFilterEngine(name)
	if !ok {
		t.Fatal("engine lost")
	}
	if got := fn(nil); len(got) != 1 || got[0].Title != "first" {
		t.Fatalf("engine = %+v, want the incumbent", got)
	}
}

func TestDuplicateFilterKeywordsSkipped(t *testing.T) {
	const name = "collidekeywords"

	plugin.Register(plugin.Descriptor{ID: "collide.kwowner", Kind: plugin.KindSignal, Signal: "collidekwowner"})
	plugin.RegisterFilterKeywords("collide.kwowner", name, func() map[string]string {
		return map[string]string{"who": "first"}
	})
	plugin.RegisterFilterKeywords("collide.kwowner", name, func() map[string]string {
		return map[string]string{"who": "second"}
	})

	kw, ok := plugin.LookupFilterKeywords(name)
	if !ok {
		t.Fatal("keywords lost")
	}
	if kw["who"] != "first" {
		t.Fatalf("keywords = %+v, want the incumbent", kw)
	}
}

func TestCollisionsDoNotStopLaterRegistrations(t *testing.T) {
	const signal = "collidelater"
	plugin.Register(plugin.Descriptor{ID: "collide.later.a", Kind: plugin.KindSignal, Signal: signal})
	plugin.Register(plugin.Descriptor{ID: "collide.later.b", Kind: plugin.KindSignal, Signal: signal})
	plugin.Register(plugin.Descriptor{
		ID:     "collide.later.c",
		Kind:   plugin.KindSignal,
		Signal: "collidelaterok",
	})
	if _, ok := plugin.Lookup("collide.later.c"); !ok {
		t.Fatal("registrations after a collision must still land")
	}
}

func TestPendingActionKindsStillFlushAfterCollisions(t *testing.T) {
	const signal = "collidepending"

	plugin.RegisterAction(signal, "later", func(context.Context, map[string]string) error { return nil })
	if _, ok := plugin.ByKind(plugin.KindAction, signal+"/later"); ok {
		t.Fatal("action kind must stay pending until its signal registers")
	}

	const dupSignal = "collidependingdup"
	plugin.Register(plugin.Descriptor{ID: "collide.pending.owner", Kind: plugin.KindSignal, Signal: dupSignal})
	plugin.Register(plugin.Descriptor{ID: "collide.pending.dup", Kind: plugin.KindSignal, Signal: dupSignal})
	if d, ok := plugin.BySignal(dupSignal); !ok || d.ID != "collide.pending.owner" {
		t.Fatalf("the local collision did not resolve to its first owner: %+v ok=%v", d, ok)
	}

	plugin.Register(plugin.Descriptor{
		ID:           "collide.pending",
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapAction},
	})

	d, ok := plugin.ByKind(plugin.KindAction, signal+"/later")
	if !ok {
		t.Fatal("pending action kind never flushed")
	}
	if d.Parent != "collide.pending" {
		t.Fatalf("action kind parent = %q", d.Parent)
	}
}
