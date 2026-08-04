package app

import (
	"context"
	"testing"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/state"
)

func activeRoleState(t *testing.T, home string) string {
	t.Helper()
	st := state.New(config.DataPath(home, config.StateDB))
	defer st.Close()
	name, _ := st.ActiveRole(context.Background())
	return name
}

func seedActiveRoleState(t *testing.T, home, name string) {
	t.Helper()
	st := state.New(config.DataPath(home, config.StateDB))
	defer st.Close()
	if err := st.SetActiveRole(context.Background(), name); err != nil {
		t.Fatal(err)
	}
}
