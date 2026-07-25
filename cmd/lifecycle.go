package cmd

import (
	"fmt"
	"os"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/render"
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
		Long:              "Creates the munin home (default ~/.munin) and seeds stock config/directives.\nDoes not require an existing config file. Override the target with --home/--dir or MUNIN_HOME.",
		Args:              cobra.NoArgs,
		Annotations:       map[string]string{annoSkipOnboarding: "true"},
		PersistentPreRunE: lifecyclePreRun,
		RunE: func(cmd *cobra.Command, _ []string) error {
			created, err := app.Install(lifecycleHome, force)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), render.Success(fmt.Sprintf("installed munin in %s:", lifecycleHome)))
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
			return app.Clean(cmd.OutOrStdout(), lifecycleHome)
		},
	}
	return c
}

func newNukeCmd() *cobra.Command {
	var yes bool
	c := &cobra.Command{
		Use:               "nuke",
		Short:             "Delete the config directory (including DuckDB)",
		Long:              "Permanently deletes the munin home directory. Does not reinstall — run `munin install` afterward.\nWith no --home/--dir/MUNIN_HOME, a matching settings.yaml home: override is cleared so install defaults to ~/.munin.",
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
		ok, err := deck.Confirm("Nuke config directory?",
			fmt.Sprintf("Permanently delete %s and ALL contents (config, queries, filters, DuckDB)?", home),
			"Delete", "Cancel")
		if err != nil {
			return err
		}
		if !ok {
			return errs.New(errs.KindUsage, "aborted")
		}
	}
	return app.Nuke(cmd.OutOrStdout(), home)
}
