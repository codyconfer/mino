package cmd

import (
	"fmt"
	"os"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/deck"
	"github.com/codyconfer/mino/internal/errs"
)

var lifecycleHome string

func lifecyclePreRun(*cobra.Command, []string) error {
	h, err := config.Home(flagHome)
	if err != nil {
		return err
	}
	lifecycleHome = h
	return nil
}

func newInstallCmd() *cobra.Command {
	var force bool
	c := &cobra.Command{
		Use:               "install",
		Short:             "Create the config directory and initialize it with defaults",
		Long:              "Creates the mino home (default ~/.mino) and seeds stock config/directives.\nDoes not require an existing config file. Override the target with --home/--dir or MINO_HOME.",
		Args:              cobra.NoArgs,
		Annotations:       map[string]string{annoSkipOnboarding: "true"},
		PersistentPreRunE: lifecyclePreRun,
		RunE: func(cmd *cobra.Command, _ []string) error {
			created, err := app.Install(lifecycleHome, force)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), Scope().Success(fmt.Sprintf("installed mino in %s:", lifecycleHome)))
			for _, p := range created {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", p)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&force, "force", false, "overwrite an existing config directory")
	return c
}

func newCleanCmd() *cobra.Command {
	c := &cobra.Command{
		Use:               "clean",
		Short:             "Archive config, flight, and query files into .archive/<timestamp>/",
		Args:              cobra.NoArgs,
		Annotations:       map[string]string{annoSkipOnboarding: "true"},
		PersistentPreRunE: lifecyclePreRun,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return app.Clean(cmd.OutOrStdout(), Scope(), lifecycleHome)
		},
	}
	return c
}

func newNukeCmd() *cobra.Command {
	var yes bool
	c := &cobra.Command{
		Use:               "nuke",
		Short:             "Delete the config directory (including DuckDB)",
		Long:              "Permanently deletes the mino home directory. Does not reinstall — run `mino install` afterward.\nWith no --home/--dir/MINO_HOME, a matching settings.yaml home: override is cleared so install defaults to ~/.mino.",
		Args:              cobra.NoArgs,
		Annotations:       map[string]string{annoSkipOnboarding: "true"},
		PersistentPreRunE: lifecyclePreRun,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runNuke(cmd, lifecycleHome, yes)
		},
	}
	c.Flags().BoolVar(&yes, "yes", false, "skip the confirmation prompt")
	return c
}

func runNuke(cmd *cobra.Command, home string, yes bool) error {
	if !yes {
		if !term.IsTerminal(os.Stdout.Fd()) {
			return errs.New(errs.KindUsage, "refusing to nuke without --yes (no terminal for confirmation)").
				WithHint("pass --yes to skip the confirmation prompt")
		}
		ok, err := deck.Confirm(vkdeck.ConfirmSpec{
			Title:    "Nuke config directory?",
			Message:  fmt.Sprintf("Permanently delete %s and ALL contents (config, queries, filters, DuckDB)?", home),
			YesLabel: "Delete",
			NoLabel:  "Cancel",
		})
		if err != nil {
			return err
		}
		if !ok {
			return errs.New(errs.KindUsage, "aborted")
		}
	}
	return app.Nuke(cmd.OutOrStdout(), Scope(), home)
}
