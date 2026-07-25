package app

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/codyconfer/sisyphus/lifecycle"
)

func TestMergeFileSeedsOverlayWins(t *testing.T) {
	overlay := fstest.MapFS{
		"config.yaml":              &fstest.MapFile{Data: []byte("output: json\n")},
		"queries/extra.yaml":       &fstest.MapFile{Data: []byte("name: extra\nsignal: demo\n")},
		"queries/my-open-prs.yaml": &fstest.MapFile{Data: []byte("name: my-open-prs\nsignal: github\n")},
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
			if string(f.Content) != "name: my-open-prs\nsignal: github\n" {
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
		"filters/demo.yaml",
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
		!strings.Contains(got["queries/demo.yaml"], "filters: [demo]") {
		t.Fatalf("demo query = %q", got["queries/demo.yaml"])
	}
	if strings.Contains(got["queries/demo.yaml"], "signal: demo") {
		t.Fatalf("demo query must not be synthetic: %q", got["queries/demo.yaml"])
	}
	if !strings.Contains(got["filters/demo.yaml"], "meta.author") {
		t.Fatalf("demo filter = %q", got["filters/demo.yaml"])
	}
	if !strings.Contains(got["demo.yaml"], "flights: [demo]") ||
		!strings.Contains(got["demo.yaml"], "demo-reviews") ||
		!strings.Contains(got["demo.yaml"], "filters: [demo]") {
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
