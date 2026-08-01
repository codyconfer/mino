package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/codyconfer/sisyphus/lifecycle"

	"github.com/codyconfer/mino/app/defaults"
	"github.com/codyconfer/mino/internal/config"
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

func TestStockSeedsIncludeDefaultHomeFlight(t *testing.T) {
	SetDefaultsFS(nil)
	spec := installSpec(t.TempDir(), true)
	got := map[string]string{}
	for _, f := range spec.Files {
		got[f.RelPath] = string(f.Content)
	}
	for _, want := range []string{
		"queries/no-bots.yaml",
		"flights/default.yaml",
		"default.yaml",
		"queries/my-open-prs.yaml",
		"queries/sisyphus-open-prs.yaml",
		"queries/sisyphus-ci.yaml",
		"queries/viewkit-open-prs.yaml",
		"queries/viewkit-ci.yaml",
		"queries/mino-open-prs.yaml",
		"queries/mino-ci.yaml",
	} {
		if _, ok := got[want]; !ok {
			t.Fatalf("missing stock seed %s (have %#v)", want, got)
		}
	}
	if !strings.Contains(got["queries/no-bots.yaml"], "meta.author") {
		t.Fatalf("no-bots filter = %q", got["queries/no-bots.yaml"])
	}
	if !strings.Contains(got["default.yaml"], "home: default") ||
		!strings.Contains(got["default.yaml"], "flights: [default]") {
		t.Fatalf("default role is not wired to the default home flight: %q", got["default.yaml"])
	}
	for _, query := range []string{"sisyphus-open-prs", "sisyphus-ci", "viewkit-open-prs", "viewkit-ci", "mino-open-prs", "mino-ci"} {
		if !strings.Contains(got["flights/default.yaml"], query) {
			t.Errorf("default flight missing %s: %q", query, got["flights/default.yaml"])
		}
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
	defaultRole, ok := dirs.Roles["default"]
	if !ok || defaultRole.Home != "default" || len(defaultRole.Flights) != 1 || defaultRole.Flights[0] != "default" {
		t.Fatalf("default role is not wired to its home flight: %#v", defaultRole)
	}
	for _, name := range dirs.FlightNames() {
		for _, q := range dirs.Flights[name].Queries {
			if _, ok := dirs.Queries[q]; !ok {
				t.Errorf("flight %q references unknown query %q", name, q)
			}
		}
	}
}

func TestEmbeddedDefaultsFSMatchesStockSeeds(t *testing.T) {
	SetDefaultsFS(nil)
	stock := installSpec(t.TempDir(), true).Files
	SetDefaultsFS(defaults.FS)
	t.Cleanup(func() { SetDefaultsFS(nil) })
	wired := installSpec(t.TempDir(), true).Files
	if len(wired) != len(stock) {
		t.Fatalf("wired seed count = %d, want %d (stock=%v wired=%v)", len(wired), len(stock), seedPaths(stock), seedPaths(wired))
	}
	for i := range stock {
		if wired[i].RelPath != stock[i].RelPath {
			t.Fatalf("seed %d: RelPath = %q, want %q", i, wired[i].RelPath, stock[i].RelPath)
		}
		if !bytes.Equal(wired[i].Content, stock[i].Content) {
			t.Fatalf("seed %s: embedded content drifted from inline seed:\n--- inline ---\n%s\n--- embedded ---\n%s",
				stock[i].RelPath, stock[i].Content, wired[i].Content)
		}
	}
}

func seedPaths(files []lifecycle.FileSeed) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.RelPath
	}
	return out
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
