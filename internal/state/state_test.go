package state

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/codyconfer/mino/internal/errs"
)

func TestActiveRoleDistinguishesUnsetFromCleared(t *testing.T) {
	ctx := context.Background()
	s := New(filepath.Join(t.TempDir(), "state.duckdb"))
	defer s.Close()

	if name, set := s.ActiveRole(ctx); set || name != "" {
		t.Fatalf("fresh store = %q, %v; want unset", name, set)
	}

	if err := s.SetActiveRole(ctx, "triage"); err != nil {
		t.Fatal(err)
	}
	if name, set := s.ActiveRole(ctx); !set || name != "triage" {
		t.Fatalf("after set = %q, %v; want triage, true", name, set)
	}

	if err := s.SetActiveRole(ctx, ""); err != nil {
		t.Fatal(err)
	}
	name, set := s.ActiveRole(ctx)
	if !set {
		t.Fatal("a cleared role must stay recorded, or the config default would come back")
	}
	if name != "" {
		t.Fatalf("cleared role = %q, want empty", name)
	}
}

func TestActiveRoleSurvivesReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.duckdb")

	s := New(path)
	if err := s.SetActiveRole(ctx, "weekly"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	again := New(path)
	defer again.Close()
	if name, set := again.ActiveRole(ctx); !set || name != "weekly" {
		t.Fatalf("reopened = %q, %v; want weekly, true", name, set)
	}
}

func TestUnavailableStoreDegrades(t *testing.T) {
	ctx := context.Background()

	var nilStore *Store
	if name, set := nilStore.ActiveRole(ctx); set || name != "" {
		t.Errorf("nil store = %q, %v; want unset", name, set)
	}
	if err := nilStore.Close(); err != nil {
		t.Errorf("closing a nil store = %v", err)
	}

	empty := New("")
	if name, set := empty.ActiveRole(ctx); set || name != "" {
		t.Errorf("pathless store = %q, %v; want unset", name, set)
	}
	err := empty.SetActiveRole(ctx, "triage")
	if err == nil {
		t.Fatal("writing to a pathless store should fail loudly, not silently drop the role")
	}
	if errs.KindOf(err) != errs.KindStore {
		t.Errorf("kind = %v, want KindStore", errs.KindOf(err))
	}

	closed := New(filepath.Join(t.TempDir(), "state.duckdb"))
	if err := closed.SetActiveRole(ctx, "triage"); err != nil {
		t.Fatal(err)
	}
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}
	if _, set := closed.ActiveRole(ctx); set {
		t.Error("a closed store should report no role, not stale data")
	}
	if err := closed.SetActiveRole(ctx, "weekly"); err == nil {
		t.Error("writing after close should fail")
	}
}

func TestOpenReportsAFailedPath(t *testing.T) {
	dir := t.TempDir()
	if _, err := Open(context.Background(), dir); err == nil {
		t.Fatal("opening a directory as the state store should fail")
	} else if errs.KindOf(err) != errs.KindStore {
		t.Errorf("kind = %v, want KindStore", errs.KindOf(err))
	}
}
