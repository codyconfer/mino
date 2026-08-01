package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/filter"
)

func TestSaveDirectiveWritesFileAndStore(t *testing.T) {
	home := t.TempDir()
	mgr, err := OpenStore(context.Background(), home)
	if err != nil {
		t.Skipf("config store unavailable: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close() })

	q := Query{Name: "built-prs", Signal: "github", Params: map[string]string{"query": "is:open is:pr"}}
	path, stored, err := SaveDirective(mgr, home, "", TypeQuery, q.Name, q)
	if err != nil {
		t.Fatal(err)
	}
	if !stored {
		t.Fatal("SaveDirective reported the directive was not stored")
	}
	if want := filepath.Join(home, DirQueries, "built-prs.yaml"); path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if !strings.Contains(string(raw), "type: query") {
		t.Errorf("SaveDirective must stamp the type: %q", raw)
	}

	cur, ok, err := mgr.Current(context.Background(), DirectivesDirective)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("directives have no current version in the store")
	}
	s, err := ParseDirectives([]byte(cur.Content))
	if err != nil {
		t.Fatalf("stored blob does not parse: %v", err)
	}
	got, ok := s.Queries["built-prs"]
	if !ok {
		t.Fatalf("stored directives missing built-prs: %v", s.QueryNames())
	}
	if got.Signal != "github" || got.Params["query"] != "is:open is:pr" {
		t.Errorf("stored query = %#v", got)
	}
	if rel := s.Source(TypeQuery, "built-prs"); rel != DirQueries+"/built-prs.yaml" {
		t.Errorf("Source = %q, want the home-relative path", rel)
	}
}

func TestSaveDirectiveHonoursAnExplicitRelPath(t *testing.T) {
	home := t.TempDir()
	q := Query{Name: "prs", Signal: "github"}
	path, _, err := SaveDirective(nil, home, "team/gh/prs.yaml", TypeQuery, q.Name, q)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "team", "gh", "prs.yaml"); path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("nested parents not created: %v", err)
	}
	s, err := LoadDirectivesFromFiles(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Queries["prs"]; !ok {
		t.Fatalf("saved directive not loadable: %v", s.QueryNames())
	}
}

func TestSaveDuckDBDirectiveUsesItsOwnCollection(t *testing.T) {
	home := t.TempDir()
	q := DuckDBQuery{Name: "recent-runs", Database: "audit", SQL: "SELECT * FROM runs"}
	path, _, err := SaveDirective(nil, home, "", TypeDuckDB, q.Name, q)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, DirDuckDB, "recent-runs.yaml"); path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	dirs, err := LoadDirectivesFromFiles(home)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := dirs.DuckDB["recent-runs"]
	if !ok || got.Database != "audit" || got.SQL != "SELECT * FROM runs" {
		t.Fatalf("loaded DuckDB query = %#v", got)
	}
	if rel := dirs.Source(TypeDuckDB, q.Name); rel != DirDuckDB+"/recent-runs.yaml" {
		t.Fatalf("source = %q", rel)
	}
}

func TestSaveDirectiveRejectsBadRelPath(t *testing.T) {
	home := t.TempDir()
	for _, rel := range []string{"../escape.yaml", "config.yaml", ".plugins/prs.yaml"} {
		if _, _, err := SaveDirective(nil, home, rel, TypeQuery, "prs", Query{Name: "prs", Signal: "github"}); err == nil {
			t.Errorf("want error saving to %q", rel)
		}
	}
	if rels, err := DirectiveFiles(home); err != nil || len(rels) != 0 {
		t.Fatalf("rejected saves wrote %v (err=%v)", rels, err)
	}
}

func TestSaveDirectiveWithoutStoreStillWritesFile(t *testing.T) {
	home := t.TempDir()
	f := Query{Name: "no-bots", Rules: []filter.Rule{{Exclude: "bot$"}}}
	path, stored, err := SaveDirective(nil, home, "", TypeFilter, f.Name, f)
	if err != nil {
		t.Fatal(err)
	}
	if stored {
		t.Error("stored should be false with no manager")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not written: %v", err)
	}
}

func TestSyncDirectivesWithoutManagerIsNoop(t *testing.T) {
	stored, err := SyncDirectives(nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if stored {
		t.Error("stored should be false with no manager")
	}
}
