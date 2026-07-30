package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func symlinkedHome(t *testing.T) (home, outside string) {
	t.Helper()
	home = t.TempDir()
	outside = t.TempDir()
	if err := os.Symlink(outside, filepath.Join(home, "queries")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	return home, outside
}

func TestSeedTargetRejectsSymlinkedParent(t *testing.T) {
	home, _ := symlinkedHome(t)
	if abs, err := seedTarget(home, "queries/evil.yaml"); err == nil {
		t.Fatalf("seedTarget = %q, nil; a symlinked parent escapes the munin home", abs)
	}
}

func TestWriteSeedsRefusesToEscapeViaSymlink(t *testing.T) {
	home, outside := symlinkedHome(t)
	seeds := []FileSeed{{RelPath: "queries/evil.yaml", Content: []byte("evil\n")}}

	written, _, err := writeSeeds(home, seeds, InstallOptions{})
	if err == nil {
		t.Errorf("writeSeeds reported success (%v) writing through a symlink out of the munin home", written)
	}
	if _, err := os.Stat(filepath.Join(outside, "evil.yaml")); err == nil {
		t.Fatal("writeSeeds wrote a file outside the munin home through a symlink")
	}
}

func TestRemoveSeedsRefusesToEscapeViaSymlink(t *testing.T) {
	home, outside := symlinkedHome(t)
	victim := filepath.Join(outside, "victim.yaml")
	if err := os.WriteFile(victim, []byte("mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	seeds := []FileSeed{{RelPath: "queries/victim.yaml", Content: []byte("mine\n")}}

	if removed, _, err := removeSeeds(home, seeds, true); err == nil {
		t.Errorf("removeSeeds reported success (%v) deleting outside the munin home", removed)
	}
	if _, err := os.Stat(victim); err != nil {
		t.Fatalf("removeSeeds deleted a file outside the munin home through a symlink: %v", err)
	}
}

func TestSeedTargetStillAcceptsOrdinaryPaths(t *testing.T) {
	home := t.TempDir()
	abs, err := seedTarget(home, "queries/ok.yaml")
	if err != nil {
		t.Fatalf("seedTarget: %v", err)
	}
	if want := filepath.Join(home, "queries", "ok.yaml"); abs != want {
		t.Fatalf("seedTarget = %q, want %q", abs, want)
	}
	if _, err := seedTarget(home, "nested/deeper/ok.yaml"); err != nil {
		t.Fatalf("seedTarget(nested): %v", err)
	}
}
