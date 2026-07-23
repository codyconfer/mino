package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/codyconfer/munin/internal/audit"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/token"
	"github.com/codyconfer/munin/internal/ui"
	"github.com/codyconfer/sisyphus"
	sconfig "github.com/codyconfer/sisyphus/config"
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

const (
	defaultConfigYAML = `# munin configuration — see the README for all options.
# Flights and roles are their own directive collections (flights/, roles/).
output: terminal

audit:
  enabled: true
`
	sampleQueryYAML = `name: my-open-prs
signal: github
params:
  query: "is:open is:pr author:@me"
`
	sampleFilterYAML = `name: no-bots
rules:
  - field: meta.author
    exclude: "(?i)bot$"
`
	sampleFlightYAML = `name: default
queries: [my-open-prs]
`
)

func newInstallCmd() *cobra.Command {
	var force bool
	c := &cobra.Command{
		Use:               "install",
		Short:             "Create the config directory and initialize it with defaults",
		Args:              cobra.NoArgs,
		PersistentPreRunE: lifecyclePreRun,
		RunE: func(cmd *cobra.Command, _ []string) error {
			created, err := doInstall(lifecycleHome, force)
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

func doInstall(home string, force bool) ([]string, error) {
	if cfgExists(home) && !force {
		return nil, errs.Newf(errs.KindConfig, "%s already has a config file", home).
			WithHint("use --force to overwrite, or `munin nuke` to reinstall")
	}

	for _, d := range []string{
		home,
		filepath.Join(home, config.DirQueries),
		filepath.Join(home, config.DirFilters),
		filepath.Join(home, config.DirFlights),
		filepath.Join(home, config.DirRoles),
	} {
		if err := sconfig.EnsureDir(d); err != nil {
			return nil, err
		}
	}

	var created []string
	files := []struct{ path, content string }{
		{filepath.Join(home, "config.yaml"), defaultConfigYAML},
		{filepath.Join(home, config.DirQueries, "my-open-prs.yaml"), sampleQueryYAML},
		{filepath.Join(home, config.DirFilters, "no-bots.yaml"), sampleFilterYAML},
		{filepath.Join(home, config.DirFlights, "default.yaml"), sampleFlightYAML},
	}
	for _, f := range files {
		if !force && sconfig.IsFile(f.path) {
			continue
		}
		if _, err := sconfig.WriteItem(filepath.Dir(f.path), filepath.Base(f.path), []byte(f.content)); err != nil {
			return nil, err
		}
		created = append(created, f.path)
	}

	if mgr, err := sisyphus.Open(home, sisyphus.Options{Mode: sisyphus.ModeBoth}); err == nil {
		if raw, format, err := sconfig.ReadFile(home); err == nil && len(raw) > 0 {
			_ = mgr.DB().Import("config", raw, format)
		}
		for _, name := range []string{config.DirQueries, config.DirFilters, config.DirFlights, config.DirRoles} {
			if blob, has, err := sconfig.SerializeDir(filepath.Join(home, name)); err == nil && has {
				_ = mgr.DB().Import(name, blob, "collection")
			}
		}
		_ = mgr.Close()
		created = append(created, filepath.Join(home, "config.duckdb"))
	}
	if a, err := audit.Open(filepath.Join(home, "audit.duckdb")); err == nil {
		_ = a.Close()
		created = append(created, filepath.Join(home, "audit.duckdb"))
	}
	if tk, err := token.Open(filepath.Join(home, "tokens.duckdb")); err == nil {
		_ = tk.Close()
		created = append(created, filepath.Join(home, "tokens.duckdb"))
	}
	return created, nil
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
	created, err := doInstall(home, true)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "nuked and reinstalled %s (%d files)\n", home, len(created))
	return nil
}

func cfgExists(home string) bool {
	for _, n := range []string{"config.yaml", "config.yml", "config.json"} {
		if sconfig.IsFile(filepath.Join(home, n)) {
			return true
		}
	}
	return false
}
