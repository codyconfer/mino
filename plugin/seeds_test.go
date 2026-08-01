package plugin_test

import (
	"testing"

	"github.com/codyconfer/mino/plugin"
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

func TestRegisterSeedsRejectsEscapingRelPaths(t *testing.T) {
	bad := []string{
		"../escape.yaml",
		"../../.config/systemd/user/foo.service",
		"queries/../../escape.yaml",
		"/etc/cron.d/evil",
		`..\..\escape.yaml`,
		"..",
		".",
		"",
		"   ",
	}
	for _, rel := range bad {
		t.Run(rel, func(t *testing.T) {
			id := "test.public.seeds.escape"
			plugin.RegisterSeeds(id, []plugin.FileSeed{{RelPath: rel, Content: []byte("evil\n")}})
			t.Cleanup(func() { plugin.RegisterSeeds(id, nil) })
			if got := plugin.SeedsFor(id); len(got) != 0 {
				t.Fatalf("RegisterSeeds kept unsafe RelPath %q: %#v", rel, got)
			}
			for _, x := range plugin.SeedPluginIDs() {
				if x == id {
					t.Fatalf("SeedPluginIDs lists a plugin whose only seed was unsafe (%q)", rel)
				}
			}
		})
	}
}

func TestRegisterSeedsKeepsSafeSeedsBesideUnsafeOnes(t *testing.T) {
	id := "test.public.seeds.mixed"
	plugin.RegisterSeeds(id, []plugin.FileSeed{
		{RelPath: "../escape.yaml", Content: []byte("evil\n")},
		{RelPath: "queries/ok.yaml", Content: []byte("ok\n")},
	})
	t.Cleanup(func() { plugin.RegisterSeeds(id, nil) })
	got := plugin.SeedsFor(id)
	if len(got) != 1 || got[0].RelPath != "queries/ok.yaml" {
		t.Fatalf("SeedsFor = %#v", got)
	}
	// Keeping the safe seed is right; dropping the unsafe one silently is not.
	findDiagnostic(t, id, "../escape.yaml")
}

func TestRegisterSeedsReportsUnsafePaths(t *testing.T) {
	id := "test.public.seeds.diag"
	plugin.RegisterSeeds(id, []plugin.FileSeed{
		{RelPath: "../escape.yaml", Content: []byte("evil\n")},
	})
	t.Cleanup(func() { plugin.RegisterSeeds(id, nil) })

	findDiagnostic(t, id, "../escape.yaml")
	findDiagnostic(t, id, "no seeds")
}
