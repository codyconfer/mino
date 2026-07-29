package cmd

import (
	"github.com/spf13/cobra"

	"github.com/codyconfer/sisyphus/configdb"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
)

func storeDB() (*configdb.Store, error) {
	if shared.Mgr == nil {
		return nil, errs.New(errs.KindStore, "store DB unavailable")
	}
	db := shared.Mgr.DB()
	if db == nil {
		return nil, errs.New(errs.KindStore, "store DB unavailable")
	}
	return db, nil
}

func validateDirectiveArg(name string) error {
	return config.ValidateDirectiveArg(name)
}

func newExportCmd() *cobra.Command {
	var out string
	var includeSecrets bool
	c := &cobra.Command{
		Use:   "export <directive>",
		Short: "Write a directive's current version from the DuckDB store to files",
		Long: "Materializes the current version of a directive held in the DuckDB store back\n" +
			"onto disk. <directive> is one of: config, directives, all.\n" +
			"config is written as config.yaml/config.json at the root; directives are\n" +
			"restored at the home-relative paths they were imported from, nesting included,\n" +
			"creating parent directories as needed. Defaults to the munin home directory.\n" +
			"queries, flights, and roles are deprecated aliases for directives.\n" +
			"Secret values in config are masked unless --include-secrets is set.",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeDirectives,
		Annotations:       map[string]string{annoReconcile: "ignore"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDirectiveArg(args[0]); err != nil {
				return err
			}
			db, err := storeDB()
			if err != nil {
				return err
			}
			home := out
			if home == "" {
				home = shared.Cfg.Home
			}
			return config.Export(cmd.OutOrStdout(), db, home, args[0], includeSecrets)
		},
	}
	c.Flags().StringVar(&out, "out", "", "output directory (default: munin home)")
	bindFlagCompletion(c, "out", completeDirs)
	c.Flags().BoolVar(&includeSecrets, "include-secrets", false, "write secret values in cleartext (default: masked; cleartext is not safe to share)")
	return c
}

func newImportCmd() *cobra.Command {
	c := &cobra.Command{
		Use:     "apply [directive]",
		Aliases: []string{"import"},
		Short:   "Apply staged config changes: write on-disk files into the DuckDB store",
		Long: "Reads the root config file and every directive file found anywhere under the\n" +
			"munin home directory, and writes them into the DuckDB store as a new current\n" +
			"version (archiving any prior version). Directives are stored keyed by their\n" +
			"home-relative path, so a later export round-trips the same layout. Never\n" +
			"prompts, so it is safe in scripts and hooks.\n" +
			"[directive] is one of: config, directives, all (default all); queries, flights,\n" +
			"and roles are deprecated aliases for directives.",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeDirectives,
		Annotations:       map[string]string{annoReconcile: "session"},
		RunE: func(cmd *cobra.Command, args []string) error {
			directive := "all"
			if len(args) == 1 {
				directive = args[0]
			}
			if err := validateDirectiveArg(directive); err != nil {
				return err
			}
			db, err := storeDB()
			if err != nil {
				return err
			}
			return config.Import(cmd.OutOrStdout(), db, shared.Cfg.Home, directive)
		},
	}
	return c
}
