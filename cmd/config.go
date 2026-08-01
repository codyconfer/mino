package cmd

import (
	"github.com/spf13/cobra"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
)

func newConfigCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "config",
		Short: "Show the active config (from the DuckDB config store) and its history",
		Long: "mino's config is stored in DuckDB: on startup the config file is hashed\n" +
			"and, when changed, imported as the new current (the prior version archived).\n" +
			"`mino config` prints the active config; `config history` lists prior versions.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if shared.Mgr == nil {
				return errs.New(errs.KindStore, "config DB unavailable")
			}
			return config.PrintCurrentConfig(cmd.OutOrStdout(), shared.Mgr.DB())
		},
	}

	c.AddCommand(&cobra.Command{
		Use:   "history",
		Short: "List archived config versions (newest first)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if shared.Mgr == nil {
				return errs.New(errs.KindStore, "config DB unavailable")
			}
			return config.PrintConfigHistory(cmd.OutOrStdout(), shared.Mgr.DB())
		},
	})
	return c
}
