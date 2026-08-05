package views

import (
	"testing"

	"github.com/codyconfer/mino/internal/app"
)

func closeDBs(t *testing.T, a *app.App) {
	t.Helper()
	t.Cleanup(a.CloseDBs)
}
