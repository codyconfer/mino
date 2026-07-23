package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/codyconfer/sisyphus"

	"github.com/codyconfer/munin/internal/errs"
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
			return runRestore(cmd, args[0], dest)
		},
	}
	c.Flags().StringVar(&dest, "dest", "", "destination directory (default: munin home)")
	return c
}

func runRestore(cmd *cobra.Command, file, dest string) error {
	sealed, err := os.ReadFile(file)
	if err != nil {
		return errs.Wrap(errs.KindBackup, err, "reading backup file")
	}
	if dest == "" {
		dest = shared.cfg.Home
	}

	closeDBs()

	names, storeName, err := sisyphus.Restore(sisyphus.RestoreSpec{
		Sealed:        sealed,
		SecretBackend: shared.cfg.Backup.SecretBackend,
		SecretName:    shared.cfg.Backup.SecretName,
		SecretService: secretService,
		DestDir:       dest,
	})
	if err != nil {
		return errs.Wrap(errs.KindBackup, err, "restoring backup").
			WithHint("ensure the same secret backend (backup.secret_backend) that created the backup is configured")
	}
	fmt.Fprintf(cmd.OutOrStdout(), "restored %s into %s (key from %s)\n",
		strings.Join(names, ", "), dest, storeName)
	return nil
}
