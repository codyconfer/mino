package plugin_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/codyconfer/sisyphus/store"

	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/plugin/ntr"
)

func TestDataPathsJoinsOpenBackupPaths(t *testing.T) {
	home := t.TempDir()
	want := filepath.Join(home, "ntr.duckdb")
	found := false
	for _, p := range plugin.DataPaths(home) {
		if p == want {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("DataPaths(%q) missing %q: %v", home, want, plugin.DataPaths(home))
	}

	st, err := ntr.Open(context.Background(), home, "default")
	if err != nil {
		t.Fatal(err)
	}
	st.Close()

	openPaths := store.BackupPaths()
	foundOpen := false
	for _, p := range openPaths {
		if p == want {
			foundOpen = true
			break
		}
	}
	if !foundOpen {
		t.Fatalf("after Open, store.BackupPaths missing %q: %v", want, openPaths)
	}
}
