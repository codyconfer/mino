package cmd

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestPruneLocalBackups(t *testing.T) {
	dir := t.TempDir()
	names := []string{
		"munin-backup-20260101-000000.tar.enc",
		"munin-backup-20260102-000000.tar.enc",
		"munin-backup-20260103-000000.tar.enc",
		"munin-backup-20260104-000000.tar.enc",
		"unrelated.txt",
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if got := pruneLocalBackups(dir, 0); got != nil {
		t.Errorf("keep=0 should prune nothing, got %v", got)
	}

	deleted := pruneLocalBackups(dir, 2)
	sort.Strings(deleted)
	if len(deleted) != 2 ||
		deleted[0] != "munin-backup-20260101-000000.tar.enc" ||
		deleted[1] != "munin-backup-20260102-000000.tar.enc" {
		t.Fatalf("deleted = %v, want the two oldest", deleted)
	}

	for _, keep := range []string{
		"munin-backup-20260103-000000.tar.enc",
		"munin-backup-20260104-000000.tar.enc",
		"unrelated.txt",
	} {
		if _, err := os.Stat(filepath.Join(dir, keep)); err != nil {
			t.Errorf("%s should have survived: %v", keep, err)
		}
	}
}
