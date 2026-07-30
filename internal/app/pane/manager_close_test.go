package pane

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/codyconfer/munin/internal/tmux"
)

type tmuxStub struct {
	splits int
	killed []tmux.PaneID
}

func stubTmux(t *testing.T, split func(tmux.SplitOpts) (tmux.PaneID, error)) *tmuxStub {
	t.Helper()
	st := &tmuxStub{}
	oldSplit, oldKill, oldExists := splitFn, killFn, existsFn
	t.Cleanup(func() { splitFn, killFn, existsFn = oldSplit, oldKill, oldExists })
	splitFn = func(o tmux.SplitOpts) (tmux.PaneID, error) {
		st.splits++
		if split != nil {
			return split(o)
		}
		return "%42", nil
	}
	killFn = func(id tmux.PaneID) error {
		st.killed = append(st.killed, id)
		return nil
	}
	existsFn = func(tmux.PaneID) bool { return true }
	return st
}

func TestCloseAllDuringSplitDropsThePane(t *testing.T) {
	snap := filepath.Join(t.TempDir(), "snap.json")
	if err := os.WriteFile(snap, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := &Manager{}
	st := stubTmux(t, func(tmux.SplitOpts) (tmux.PaneID, error) {
		m.CloseAll()
		return "%42", nil
	})

	if err := m.split(tmux.SplitOpts{}, snap); err == nil {
		t.Fatal("split racing CloseAll reported success, so the caller believes the pane is tracked")
	}

	m.mu.Lock()
	n := len(m.panes)
	m.mu.Unlock()
	if n != 0 {
		t.Fatalf("tracked panes = %d after CloseAll: the pane is orphaned and nothing ever kills it", n)
	}
	if len(st.killed) == 0 {
		t.Fatal("pane opened during CloseAll was never killed")
	}
	if _, err := os.Stat(snap); !os.IsNotExist(err) {
		t.Fatalf("snapshot %s survived the dropped pane: %v", snap, err)
	}
}

func TestSplitAfterCloseAllIsRejected(t *testing.T) {
	m := &Manager{}
	st := stubTmux(t, nil)
	m.CloseAll()

	if err := m.split(tmux.SplitOpts{}, ""); err == nil {
		t.Fatal("split after CloseAll reported success")
	}
	if st.splits != 0 {
		t.Fatalf("tmux.Split called %d times after CloseAll, want 0", st.splits)
	}
	m.mu.Lock()
	n := len(m.panes)
	m.mu.Unlock()
	if n != 0 {
		t.Fatalf("tracked panes = %d after CloseAll, want 0", n)
	}
}

func TestSplitTracksWhileOpen(t *testing.T) {
	m := &Manager{}
	stubTmux(t, nil)
	if err := m.split(tmux.SplitOpts{}, ""); err != nil {
		t.Fatalf("split: %v", err)
	}
	m.mu.Lock()
	n := len(m.panes)
	m.mu.Unlock()
	if n != 1 {
		t.Fatalf("tracked panes = %d, want 1", n)
	}
}
