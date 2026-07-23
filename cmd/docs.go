package cmd

import (
	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/signals"
	"github.com/codyconfer/munin/internal/signals/gdocs"
)

func newDocsCmd() *cobra.Command {
	return sourceCmd("docs", "Recently modified Google Docs")
}

func buildDocs(params map[string]string) (signals.Signal, error) {
	recent := paramInt(params, "recent", shared.cfg.Docs.Recent)
	return gdocs.New(recent, googleAuth()), nil
}
