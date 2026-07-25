package cmd

import (
	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/app/backup"
)

func newBackupCmd() *cobra.Command {
	var out string
	c := &cobra.Command{
		Use:   "backup",
		Short: "Create an encrypted backup of munin's DuckDB databases",
		Long: "Bundles the DuckDB files under <home>/.data (config, audit, tokens) into a\n" +
			"tar, encrypts it with AES-256-GCM, and writes it to the current directory (or the app's\n" +
			"private Google Drive folder when backup.destination=gdrive). The encryption\n" +
			"key is escrowed in your secret manager (Bitwarden or 1Password if configured,\n" +
			"otherwise the OS keyring).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return backup.Run(cmd.Context(), cmd.OutOrStdout(), shared.Cfg, closeDBs, googleAuth(), out)
		},
	}
	c.Flags().StringVar(&out, "out", ".", "output directory for a local backup")
	return c
}

func closeDBs() { shared.CloseDBs() }
