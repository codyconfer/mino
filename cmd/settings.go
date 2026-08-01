package cmd

import (
	"os"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/codyconfer/mino/internal/deck"
	"github.com/codyconfer/mino/internal/errs"
)

func newSettingsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "settings",
		Short: "Open the settings TUI (config, directives, DuckDB)",
		Long: "Opens only the settings screens. Invoked from the shell, quitting returns\n" +
			"to the shell (not the main menu).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !term.IsTerminal(os.Stdout.Fd()) {
				return errs.New(errs.KindUsage, "settings requires an interactive terminal")
			}
			return deck.Run(buildViews().Settings(), deck.WithStatus(statusProvider()))
		},
	}
}
