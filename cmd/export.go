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
			"onto disk. <directive> is one of: config, queries, filters, flights, roles, all.\n" +
			"config is written as config.yaml/config.json; each collection is written as\n" +
			"individual files under <out>/<directive>/. Defaults to the munin home directory.\n" +
			"Secret values in config are masked unless --include-secrets is set.",
		Args: cobra.ExactArgs(1),
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
	c.Flags().BoolVar(&includeSecrets, "include-secrets", false, "write secret values in cleartext (default: masked; cleartext is not safe to share)")
	return c
}

func newImportCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "import <directive>",
		Short: "Persist on-disk files into the DuckDB store as the current version",
		Long: "Reads directive files from the munin home directory and writes them into the\n" +
			"DuckDB store as a new current version (archiving any prior version).\n" +
			"<directive> is one of: config, queries, filters, flights, roles, all.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDirectiveArg(args[0]); err != nil {
				return err
			}
			db, err := storeDB()
			if err != nil {
				return err
			}
			return config.Import(cmd.OutOrStdout(), db, shared.Cfg.Home, args[0])
		},
	}
	return c
}
