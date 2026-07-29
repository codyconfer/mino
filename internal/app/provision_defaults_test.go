package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/codyconfer/sisyphus/lifecycle"

	"github.com/codyconfer/munin/internal/config"
)

func TestMergeFileSeedsOverlayWins(t *testing.T) {
	overlay := fstest.MapFS{
		"config.yaml":              &fstest.MapFile{Data: []byte("output: json\n")},
		"queries/extra.yaml":       &fstest.MapFile{Data: []byte("name: extra\ntype: query\nsignal: demo\n")},
		"queries/my-open-prs.yaml": &fstest.MapFile{Data: []byte("name: my-open-prs\ntype: query\nsignal: github\n")},
	}
	SetDefaultsFS(overlay)
	t.Cleanup(func() { SetDefaultsFS(nil) })

	spec := installSpec(t.TempDir(), true)
	var sawConfig, sawExtra, sawPRs bool
	for _, f := range spec.Files {
		switch f.RelPath {
		case "config.yaml":
			sawConfig = true
			if string(f.Content) != "output: json\n" {
				t.Fatalf("config overlay = %q", f.Content)
			}
		case "queries/extra.yaml":
			sawExtra = true
		case "queries/my-open-prs.yaml":
			sawPRs = true
			if string(f.Content) != "name: my-open-prs\ntype: query\nsignal: github\n" {
				t.Fatalf("prs overlay = %q", f.Content)
			}
		}
	}
	if !sawConfig || !sawExtra || !sawPRs {
		t.Fatalf("missing seeds: config=%v extra=%v prs=%v files=%v", sawConfig, sawExtra, sawPRs, spec.Files)
	}
}

func TestStockSeedsIncludeOptInDemoFlight(t *testing.T) {
	SetDefaultsFS(nil)
	spec := installSpec(t.TempDir(), true)
	got := map[string]string{}
	for _, f := range spec.Files {
		got[f.RelPath] = string(f.Content)
	}
	for _, want := range []string{
		"queries/demo.yaml",
		"queries/demo-reviews.yaml",
		"queries/no-bots.yaml",
		"flights/demo.yaml",
		"demo.yaml",
		"queries/notify-smoke.yaml",
		"flights/notify-smoke.yaml",
		"flights/default.yaml",
		"queries/my-open-prs.yaml",
	} {
		if _, ok := got[want]; !ok {
			t.Fatalf("missing stock seed %s (have %#v)", want, got)
		}
	}
	if !strings.Contains(got["flights/demo.yaml"], "demo-reviews") {
		t.Fatalf("demo flight = %q", got["flights/demo.yaml"])
	}
	if !strings.Contains(got["queries/demo.yaml"], "signal: github") ||
		!strings.Contains(got["queries/demo.yaml"], "rules:") {
		t.Fatalf("demo query = %q", got["queries/demo.yaml"])
	}
	if !strings.Contains(got["queries/demo-reviews.yaml"], "filters: [no-bots]") {
		t.Fatalf("demo-reviews query = %q", got["queries/demo-reviews.yaml"])
	}
	if strings.Contains(got["queries/demo.yaml"], "signal: demo") {
		t.Fatalf("demo query must not be synthetic: %q", got["queries/demo.yaml"])
	}
	if !strings.Contains(got["queries/no-bots.yaml"], "meta.author") {
		t.Fatalf("no-bots filter = %q", got["queries/no-bots.yaml"])
	}
	if !strings.Contains(got["demo.yaml"], "flights: [demo]") ||
		!strings.Contains(got["demo.yaml"], "demo-reviews") ||
		!strings.Contains(got["demo.yaml"], "no-bots") {
		t.Fatalf("demo role = %q", got["demo.yaml"])
	}
	if !strings.Contains(got["queries/notify-smoke.yaml"], "signal: demo") {
		t.Fatalf("notify-smoke = %q", got["queries/notify-smoke.yaml"])
	}
	if strings.Contains(got["flights/default.yaml"], "demo") ||
		strings.Contains(got["flights/default.yaml"], "notify-smoke") {
		t.Fatalf("default flight must not reference demo/notify-smoke: %q", got["flights/default.yaml"])
	}
}

func TestStockSeedsLoadAsTypedDirectives(t *testing.T) {
	SetDefaultsFS(nil)
	home := t.TempDir()
	spec := installSpec(home, true)
	for _, f := range spec.Files {
		if f.RelPath == "config.yaml" {
			continue
		}
		dest := filepath.Join(home, filepath.FromSlash(f.RelPath))
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dest, f.Content, 0o600); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(f.Content), "type:") {
			t.Errorf("stock seed %s declares no type:\n%s", f.RelPath, f.Content)
		}
	}

	dirs, err := config.LoadDirectivesFromFiles(home)
	if err != nil {
		t.Fatalf("stock seeds do not load: %v", err)
	}
	if len(dirs.Queries) == 0 || len(dirs.Flights) == 0 || len(dirs.Roles) == 0 {
		t.Fatalf("stock seeds should populate every kind: q=%d fl=%d r=%d",
			len(dirs.Queries), len(dirs.Flights), len(dirs.Roles))
	}
	for _, name := range dirs.FlightNames() {
		for _, q := range dirs.Flights[name].Queries {
			if _, ok := dirs.Queries[q]; !ok {
				t.Errorf("flight %q references unknown query %q", name, q)
			}
		}
	}
}

func TestMergeFileSeedsNormalizesSeparators(t *testing.T) {
	merged := mergeFileSeeds(
		[]lifecycle.FileSeed{{RelPath: `queries\my-open-prs.yaml`, Content: []byte("stock")}},
		[]lifecycle.FileSeed{{RelPath: "queries/my-open-prs.yaml", Content: []byte("overlay")}},
	)
	if len(merged) != 1 {
		t.Fatalf("want 1 seed after slash-normalized merge, got %d: %#v", len(merged), merged)
	}
	if merged[0].RelPath != "queries/my-open-prs.yaml" {
		t.Fatalf("RelPath = %q", merged[0].RelPath)
	}
	if string(merged[0].Content) != "overlay" {
		t.Fatalf("content = %q", merged[0].Content)
	}
	for _, f := range merged {
		if strings.Contains(f.RelPath, `\`) {
			t.Fatalf("RelPath still has backslash: %q", f.RelPath)
		}
	}
}
