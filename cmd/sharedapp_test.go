package cmd

import "testing"

func closeSharedDBs(t *testing.T) {
	t.Helper()
	a := shared
	t.Cleanup(func() {
		if a != nil {
			a.CloseDBs()
		}
	})
}
