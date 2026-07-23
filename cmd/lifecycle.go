package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	sconfig "github.com/codyconfer/sisyphus/config"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/provision"
	"github.com/codyconfer/munin/internal/ui"
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
		Args:              cobra.NoArgs,
		PersistentPreRunE: lifecyclePreRun,
		RunE: func(cmd *cobra.Command, _ []string) error {
			created, err := provision.Install(lifecycleHome, force)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "installed munin in %s:\n", lifecycleHome)
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
		PersistentPreRunE: lifecyclePreRun,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runClean(cmd, lifecycleHome)
		},
	}
	return c
}

func newNukeCmd() *cobra.Command {
	var yes bool
	c := &cobra.Command{
		Use:               "nuke",
		Short:             "Delete the config directory (including DuckDB) and reinstall defaults",
		Args:              cobra.NoArgs,
		PersistentPreRunE: lifecyclePreRun,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runNuke(cmd, lifecycleHome, yes)
		},
	}
	c.Flags().BoolVar(&yes, "yes", false, "skip the confirmation prompt")
	return c
}

func runClean(cmd *cobra.Command, home string) error {
	entries := []string{
		"config.yaml", "config.yml", "config.json",
		config.DirQueries, config.DirFilters, config.DirFlights, config.DirRoles,
	}
	dest, moved, err := sconfig.Archive(home, entries)
	if err != nil {
		return err
	}
	if len(moved) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "nothing to clean (no config/query/filter files present)")
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "archived %s to %s\n", strings.Join(moved, ", "), dest)
	return nil
}

func runNuke(cmd *cobra.Command, home string, yes bool) error {
	if !yes {
		if !term.IsTerminal(int(os.Stdout.Fd())) {
			return errs.New(errs.KindUsage, "refusing to nuke without --yes (no terminal for confirmation)").
				WithHint("pass --yes to skip the confirmation prompt")
		}
		ok, err := ui.Confirm("Nuke config directory?",
			fmt.Sprintf("Permanently delete %s and ALL contents (config, queries, filters, DuckDB)?", home),
			"Delete", "Cancel")
		if err != nil {
			return err
		}
		if !ok {
			return errs.New(errs.KindUsage, "aborted")
		}
	}
	if err := sconfig.RemoveAll(home); err != nil {
		return errs.Wrapf(errs.KindInternal, err, "removing %s", home)
	}
	created, err := provision.Install(home, true)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "nuked and reinstalled %s (%d files)\n", home, len(created))
	return nil
}
