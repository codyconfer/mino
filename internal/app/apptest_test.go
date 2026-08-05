package app

import "testing"

func closeDBs(t *testing.T, a *App) {
	t.Helper()
	t.Cleanup(a.CloseDBs)
}
