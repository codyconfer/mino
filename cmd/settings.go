package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/tui"
)

func newSettingsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "settings",
		Short: "Open the settings TUI (config, directives, DuckDB)",
		Long: "Opens only the settings screens. Invoked from the shell, quitting returns\n" +
			"to the shell (not the main menu).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !term.IsTerminal(int(os.Stdout.Fd())) {
				return errs.New(errs.KindUsage, "settings requires an interactive terminal")
			}
			return tui.Run(buildViews().Settings())
		},
	}
}
