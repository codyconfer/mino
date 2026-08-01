package cmd

import (
	"strconv"

	"github.com/spf13/cobra"

	"github.com/codyconfer/mino/internal/audit"
	"github.com/codyconfer/mino/internal/errs"
)

func newHistoryCmd() *cobra.Command {
	var limit int
	c := &cobra.Command{
		Use:   "history [show <id>]",
		Short: "Recall past flights and query results from the audit trail",
		Long: "The audit trail records every flight and query run (with timestamps and\n" +
			"item counts) in DuckDB. `mino history` lists recent runs; `mino history\n" +
			"show <id>` recalls a run's stored results. This is a record of what you ran\n" +
			"over time, not a cache.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return audit.PrintRecent(cmd.OutOrStdout(), shared.Audit, limit)
		},
	}
	c.Flags().IntVar(&limit, "limit", 20, "max runs to list")

	c.AddCommand(&cobra.Command{
		Use:   "show <id>",
		Short: "Recall a past run's stored results",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return errs.Newf(errs.KindUsage, "invalid run id %q", args[0])
			}
			return audit.PrintEntry(cmd.OutOrStdout(), shared.Audit, id)
		},
	})
	return c
}
