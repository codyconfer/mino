package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/codyconfer/sisyphus"

	"github.com/codyconfer/munin/internal/backup"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/signals/gdrive"
)

const secretService = "munin"

func newBackupCmd() *cobra.Command {
	var out string
	c := &cobra.Command{
		Use:   "backup",
		Short: "Create an encrypted backup of munin's DuckDB databases",
		Long: "Bundles config.duckdb, audit.duckdb, and tokens.duckdb into a tar, encrypts\n" +
			"it with AES-256-GCM, and writes it to the current directory (or the app's\n" +
			"private Google Drive folder when backup.destination=gdrive). The encryption\n" +
			"key is escrowed in your secret manager (Bitwarden or 1Password if configured,\n" +
			"otherwise the OS keyring).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runBackup(cmd, out)
		},
	}
	c.Flags().StringVar(&out, "out", ".", "output directory for a local backup")
	return c
}

func runBackup(cmd *cobra.Command, outDir string) error {
	home := shared.cfg.Home

	closeDBs()

	sealed, storeName, err := sisyphus.Backup(sisyphus.BackupSpec{
		Files: []string{
			filepath.Join(home, "config.duckdb"),
			filepath.Join(home, "audit.duckdb"),
			filepath.Join(home, "tokens.duckdb"),
		},
		SecretBackend: shared.cfg.Backup.SecretBackend,
		SecretName:    shared.cfg.Backup.SecretName,
		SecretService: secretService,
	})
	if err != nil {
		return errs.Wrap(errs.KindBackup, err, "creating encrypted backup").
			WithHint("configure a secret backend (backup.secret_backend) or ensure the OS keyring is available")
	}

	name := fmt.Sprintf("%s%s.tar.enc", backup.Prefix, time.Now().Format("20060102-150405"))
	out := cmd.OutOrStdout()
	keep := shared.cfg.Backup.Keep

	if shared.cfg.Backup.Destination == "gdrive" {
		item, err := gdrive.UploadAppData(cmd.Context(), googleAuth(), name, sealed, "application/octet-stream")
		if err != nil {
			return errs.Wrap(errs.KindBackup, err, "uploading backup to Google Drive")
		}
		fmt.Fprintf(out, "encrypted backup uploaded to Google Drive app data: %s (key in %s)\n", item.Title, storeName)
		if deleted, err := gdrive.PruneAppData(cmd.Context(), googleAuth(), backup.Prefix, keep); err != nil {
			verbosef("backup retention: %v", err)
		} else if len(deleted) > 0 {
			fmt.Fprintf(out, "pruned %d old backup(s) (keep=%d)\n", len(deleted), keep)
		}
		return nil
	}

	path := filepath.Join(outDir, name)
	if err := os.WriteFile(path, sealed, 0o600); err != nil {
		return errs.Wrapf(errs.KindBackup, err, "writing backup to %s", path)
	}
	fmt.Fprintf(out, "encrypted backup written to %s (%d bytes; key in %s)\n", path, len(sealed), storeName)
	if pruned := backup.PruneLocal(outDir, keep); len(pruned) > 0 {
		fmt.Fprintf(out, "pruned %d old backup(s) (keep=%d)\n", len(pruned), keep)
	}
	return nil
}

func closeDBs() {
	if shared.audit != nil {
		_ = shared.audit.Close()
		shared.audit = nil
	}
	if shared.tokens != nil {
		_ = shared.tokens.Close()
		shared.tokens = nil
	}
	if shared.mgr != nil {
		_ = shared.mgr.Close()
		shared.mgr = nil
	}
}
