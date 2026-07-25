package app

import (
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/filter"
	"github.com/codyconfer/munin/internal/signals"
)

func TestDemoSeedsWireGitHubFilterAndRole(t *testing.T) {
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
		"demo.yaml",
		"flights/demo.yaml",
		"queries/notify-smoke.yaml",
		"flights/notify-smoke.yaml",
	} {
		if _, ok := got[want]; !ok {
			t.Fatalf("missing seed %s", want)
		}
	}
	if !strings.Contains(got["queries/demo.yaml"], "signal: github") {
		t.Fatalf("demo query must use github, got %q", got["queries/demo.yaml"])
	}
	if strings.Contains(got["queries/demo.yaml"], "signal: demo") {
		t.Fatalf("demo query must not use synthetic demo signal: %q", got["queries/demo.yaml"])
	}
	if !strings.Contains(got["queries/demo.yaml"], "is:open is:pr author:@me") {
		t.Fatalf("demo query missing github search: %q", got["queries/demo.yaml"])
	}
	if !strings.Contains(got["queries/demo-reviews.yaml"], "signal: github") ||
		!strings.Contains(got["queries/demo-reviews.yaml"], "review-requested:@me") {
		t.Fatalf("demo-reviews query = %q", got["queries/demo-reviews.yaml"])
	}
	if !strings.Contains(got["queries/notify-smoke.yaml"], "signal: demo") {
		t.Fatalf("notify-smoke should keep synthetic demo: %q", got["queries/notify-smoke.yaml"])
	}
	if !strings.Contains(got["flights/demo.yaml"], "demo-reviews") {
		t.Fatalf("demo flight = %q", got["flights/demo.yaml"])
	}

	home := t.TempDir()
	for _, rel := range []string{
		path.Join(config.DirQueries, "demo.yaml"),
		path.Join(config.DirQueries, "demo-reviews.yaml"),
		path.Join(config.DirFilters, "demo.yaml"),
		path.Join(config.DirFlights, "demo.yaml"),
		"demo.yaml",
	} {
		body, ok := got[rel]
		if !ok || body == "" {
			t.Fatalf("seed %s missing or empty", rel)
		}
		dest := filepath.Join(home, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dest, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	dirs, err := config.LoadDirectivesFromFiles(home)
	if err != nil {
		t.Fatalf("LoadDirectivesFromFiles: %v", err)
	}
	q, ok := dirs.Queries["demo"]
	if !ok {
		t.Fatal("missing demo query")
	}
	if q.Signal != "github" {
		t.Fatalf("demo signal = %q, want github", q.Signal)
	}
	if q.Params["query"] == "" {
		t.Fatal("demo query missing params.query")
	}
	resolved, err := dirs.Resolve(q)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(resolved) != 1 || resolved[0].Name != "demo" {
		t.Fatalf("resolved filters = %#v", resolved)
	}

	compiled, err := filter.CompileAll(resolved)
	if err != nil {
		t.Fatal(err)
	}
	items := filter.ApplyAll(compiled, []signals.Item{
		{
			Kind: "pr", Title: "Fix flaky test",
			URL:  "https://github.com/octo/munin/pull/7",
			Meta: map[string]string{"author": "alice"},
		},
		{
			Kind: "pr", Title: "CI green",
			URL:  "https://github.com/octo/munin/pull/8",
			Meta: map[string]string{"author": "deploy-bot"},
		},
	})
	if len(items) != 1 || items[0].Meta["author"] != "alice" || items[0].URL == "" {
		t.Fatalf("demo filter over github-shaped items = %#v", items)
	}

	access := config.NewAccess("demo", dirs.Roles)
	if !access.FlightVisible("demo") || !access.QueryVisible("demo") ||
		!access.QueryVisible("demo-reviews") || !access.FilterVisible("demo") {
		t.Fatalf("demo role should expose demo flight/queries/filter")
	}
	if access.FlightVisible("default") || access.QueryVisible("my-open-prs") ||
		access.QueryVisible("notify-smoke") || access.FilterVisible("no-bots") {
		t.Fatalf("demo role should hide non-demo directives")
	}
}
