package cmd

import (
	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/app/backup"
)

func newRestoreCmd() *cobra.Command {
	var dest string
	c := &cobra.Command{
		Use:   "restore <backup-file>",
		Short: "Restore DuckDB databases from an encrypted backup",
		Long: "Decrypts a `munin backup` file using the key escrowed in your secret manager\n" +
			"and writes the databases into the munin home (or --dest). Existing files are\n" +
			"overwritten, so the restored config/audit/tokens take effect on the next run.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return backup.Restore(cmd.Context(), cmd.OutOrStdout(), shared.Cfg, closeDBs, args[0], dest)
		},
	}
	c.Flags().StringVar(&dest, "dest", "", "destination directory (default: munin home)")
	return c
}
