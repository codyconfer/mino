package plugin_test

import (
	"testing"

	"github.com/codyconfer/munin/plugin"
)

func TestRegisterSeedsRoundTrip(t *testing.T) {
	id := "test.public.seeds"
	plugin.RegisterSeeds(id, []plugin.FileSeed{
		{RelPath: "queries/a.yaml", Content: []byte("a\n")},
	})
	t.Cleanup(func() { plugin.RegisterSeeds(id, nil) })

	got := plugin.SeedsFor(id)
	if len(got) != 1 || got[0].RelPath != "queries/a.yaml" || string(got[0].Content) != "a\n" {
		t.Fatalf("SeedsFor = %#v", got)
	}
	ids := plugin.SeedPluginIDs()
	found := false
	for _, x := range ids {
		if x == id {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("SeedPluginIDs missing %s: %v", id, ids)
	}

	plugin.RegisterSeeds(id, nil)
	if len(plugin.SeedsFor(id)) != 0 {
		t.Fatal("expected clear")
	}
}
