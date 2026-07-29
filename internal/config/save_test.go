package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/codyconfer/munin/internal/filter"
)

func TestSaveCollectionItemWritesFileAndStore(t *testing.T) {
	home := t.TempDir()
	mgr, err := OpenStore(context.Background(), home)
	if err != nil {
		t.Skipf("config store unavailable: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close() })

	q := Query{Name: "built-prs", Signal: "github", Params: map[string]string{"query": "is:open is:pr"}}
	path, stored, err := SaveCollectionItem(mgr, home, DirQueries, q.Name, q)
	if err != nil {
		t.Fatal(err)
	}
	if !stored {
		t.Fatal("SaveCollectionItem reported the item was not stored")
	}
	if want := filepath.Join(home, DirQueries, "built-prs.yaml"); path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not written: %v", err)
	}

	cur, ok, err := mgr.Current(context.Background(), DirQueries)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("queries collection has no current version in the store")
	}
	queries, err := ParseQueries([]byte(cur.Content))
	if err != nil {
		t.Fatalf("stored blob does not parse: %v", err)
	}
	got, ok := queries["built-prs"]
	if !ok {
		t.Fatalf("stored queries missing built-prs: %v", queries)
	}
	if got.Signal != "github" || got.Params["query"] != "is:open is:pr" {
		t.Errorf("stored query = %#v", got)
	}
}

func TestSaveCollectionItemWithoutStoreStillWritesFile(t *testing.T) {
	home := t.TempDir()
	f := Query{Name: "no-bots", Rules: []filter.Rule{{Exclude: "bot$"}}}
	path, stored, err := SaveCollectionItem(nil, home, DirQueries, f.Name, f)
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
